// Code generated by mockery v1.0.0. DO NOT EDIT.

package clustersetup

import context "context"
import helm "github.com/banzaicloud/pipeline/pkg/helm"
import mock "github.com/stretchr/testify/mock"

// MockHelmClientFactory is an autogenerated mock type for the HelmClientFactory type
type MockHelmClientFactory struct {
	mock.Mock
}

// FromSecret provides a mock function with given fields: ctx, secretID
func (_m *MockHelmClientFactory) FromSecret(ctx context.Context, secretID string) (*helm.Client, error) {
	ret := _m.Called(ctx, secretID)

	var r0 *helm.Client
	if rf, ok := ret.Get(0).(func(context.Context, string) *helm.Client); ok {
		r0 = rf(ctx, secretID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*helm.Client)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(context.Context, string) error); ok {
		r1 = rf(ctx, secretID)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
