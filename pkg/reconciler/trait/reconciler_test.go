/*
Copyright 2020 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package trait

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/resource/fake"
	"github.com/crossplane/crossplane-runtime/pkg/test"

	traitfake "github.com/crossplane/addon-oam-kubernetes-remote/pkg/reconciler/trait/fake"
)

var _ reconcile.Reconciler = &Reconciler{}

func TestReconciler(t *testing.T) {
	type args struct {
		m manager.Manager
		t Kind
		p Kind
		o []ReconcilerOption
	}

	type want struct {
		result reconcile.Result
		err    error
	}

	errBoom := errors.New("boom")

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"GetTraitError": {
			reason: "Any error (except not found) encountered while getting the resource under reconciliation should be returned.",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{MockGet: test.NewMockGetFn(errBoom)},
					Scheme: fake.SchemeWith(&traitfake.Trait{}, &traitfake.Object{}),
				},
				t: Kind(fake.GVK(&traitfake.Trait{})),
				p: Kind(fake.GVK(&traitfake.Object{})),
			},
			want: want{err: errors.Wrap(errBoom, errGetTrait)},
		},
		"TraitNotFound": {
			reason: "Not found errors encountered while getting the resource under reconciliation should be ignored.",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{MockGet: test.NewMockGetFn(kerrors.NewNotFound(schema.GroupResource{}, ""))},
					Scheme: fake.SchemeWith(&traitfake.Trait{}),
				},
				t: Kind(fake.GVK(&traitfake.Trait{})),
				p: Kind(fake.GVK(&traitfake.Object{})),
			},
			want: want{result: reconcile.Result{}},
		},
		"TranslationNotFound": {
			reason: "",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet: func(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
							if _, ok := obj.(Trait); ok {
								return nil
							}
							return kerrors.NewNotFound(schema.GroupResource{}, "")
						},
						MockStatusUpdate: func(_ context.Context, obj runtime.Object, _ ...client.UpdateOption) error {
							got := obj.(Trait)

							if diff := cmp.Diff(v1alpha1.ReasonReconcileSuccess, got.GetCondition(v1alpha1.TypeSynced).Reason); diff != "" {
								return errors.Errorf("MockStatusUpdate: -want, +got: %s", diff)
							}

							return nil
						},
					},
					Scheme: fake.SchemeWith(&traitfake.Trait{}, &traitfake.Object{}),
				},
				t: Kind(fake.GVK(&traitfake.Trait{})),
				p: Kind(fake.GVK(&traitfake.Object{})),
			},
			want: want{result: reconcile.Result{RequeueAfter: shortWait}},
		},
		"GetPackageError": {
			reason: "",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet: func(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
							if _, ok := obj.(Trait); ok {
								return nil
							}
							return errBoom
						},
						MockStatusUpdate: func(_ context.Context, obj runtime.Object, _ ...client.UpdateOption) error {
							got := obj.(Trait)

							if diff := cmp.Diff(v1alpha1.ReasonReconcileError, got.GetCondition(v1alpha1.TypeSynced).Reason); diff != "" {
								return errors.Errorf("MockStatusUpdate: -want, +got: %s", diff)
							}

							if diff := cmp.Diff(errors.Wrap(errBoom, errGetTranslation).Error(), got.GetCondition(v1alpha1.TypeSynced).Message); diff != "" {
								return errors.Errorf("MockStatusUpdate: -want, +got: %s", diff)
							}

							return nil
						},
					},
					Scheme: fake.SchemeWith(&traitfake.Trait{}, &traitfake.Object{}),
				},
				t: Kind(fake.GVK(&traitfake.Trait{})),
				p: Kind(fake.GVK(&traitfake.Object{})),
			},
			want: want{result: reconcile.Result{RequeueAfter: shortWait}},
		},
		"ModifyError": {
			reason: "",
			args: args{
				m: &fake.Manager{
					Client: &test.MockClient{
						MockGet: test.NewMockGetFn(nil),
						MockStatusUpdate: func(_ context.Context, obj runtime.Object, _ ...client.UpdateOption) error {
							got := obj.(Trait)

							if diff := cmp.Diff(v1alpha1.ReasonReconcileError, got.GetCondition(v1alpha1.TypeSynced).Reason); diff != "" {
								return errors.Errorf("MockStatusUpdate: -want, +got: %s", diff)
							}

							if diff := cmp.Diff(errors.Wrap(errBoom, errTraitModify).Error(), got.GetCondition(v1alpha1.TypeSynced).Message); diff != "" {
								return errors.Errorf("MockStatusUpdate: -want, +got: %s", diff)
							}

							return nil
						},
					},
					Scheme: fake.SchemeWith(&traitfake.Trait{}, &traitfake.Object{}),
				},
				t: Kind(fake.GVK(&traitfake.Trait{})),
				p: Kind(fake.GVK(&traitfake.Object{})),
				o: []ReconcilerOption{WithModifier(ModifyFn(func(_ context.Context, _ runtime.Object, _ Trait) error {
					return errBoom
				}))},
			},
			want: want{result: reconcile.Result{RequeueAfter: shortWait}},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := NewReconciler(tc.args.m, tc.args.t, tc.args.p, tc.args.o...)
			got, err := r.Reconcile(reconcile.Request{})

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\nReason: %s\nr.Reconcile(...): -want error, +got error:\n%s", tc.reason, diff)
			}

			if diff := cmp.Diff(tc.want.result, got); diff != "" {
				t.Errorf("\nReason: %s\nr.Reconcile(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}
