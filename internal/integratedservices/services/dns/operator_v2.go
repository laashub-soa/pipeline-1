// Copyright © 2020 Banzai Cloud
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dns

import (
	"context"

	"emperror.dev/errors"
	"github.com/banzaicloud/integrated-service-sdk/api/v1alpha1"
	"github.com/mitchellh/mapstructure"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/banzaicloud/pipeline/internal/common"
	"github.com/banzaicloud/pipeline/internal/integratedservices"
	"github.com/banzaicloud/pipeline/internal/integratedservices/integratedserviceadapter"
	"github.com/banzaicloud/pipeline/internal/integratedservices/services"
	"github.com/banzaicloud/pipeline/internal/integratedservices/services/dns/externaldns"
	"github.com/banzaicloud/pipeline/internal/secret/secrettype"
	"github.com/banzaicloud/pipeline/src/auth"
	"github.com/banzaicloud/pipeline/src/dns/route53"
)

type Operator struct {
	clusterGetter     integratedserviceadapter.ClusterGetter
	clusterService    integratedservices.ClusterService
	orgDomainService  OrgDomainService
	secretStore       services.SecretStore
	config            Config
	reconciler        Reconciler
	specWrapper       integratedservices.SpecWrapper
	serviceNameMapper services.ServiceNameMapper
	logger            common.Logger
}

func NewDNSISOperator(
	clusterGetter integratedserviceadapter.ClusterGetter,
	clusterService integratedservices.ClusterService,
	orgDomainService OrgDomainService,
	secretStore services.SecretStore,
	config Config,
	wrapper integratedservices.SpecWrapper,

	logger common.Logger,
) Operator {
	return Operator{
		clusterGetter:     clusterGetter,
		clusterService:    clusterService,
		orgDomainService:  orgDomainService,
		secretStore:       secretStore,
		config:            config,
		reconciler:        NewISReconciler(logger),
		specWrapper:       wrapper,
		serviceNameMapper: services.NewServiceNameMapper(),
		logger:            logger,
	}
}

func (o Operator) Deactivate(ctx context.Context, clusterID uint, _ integratedservices.IntegratedServiceSpec) error {
	ctx, err := o.ensureOrgIDInContext(ctx, clusterID)
	if err != nil {
		return err
	}

	if err := o.clusterService.CheckClusterReady(ctx, clusterID); err != nil {
		return err
	}

	cl, err := o.clusterGetter.GetClusterByIDOnly(ctx, clusterID)
	if err != nil {
		return errors.WrapIf(err, "failed to retrieve the cluster")
	}

	k8sConfig, err := cl.GetK8sConfig()
	if err != nil {
		return errors.WrapIf(err, "failed to retrieve the k8s config")
	}

	si := v1alpha1.ServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: o.config.Namespace,
			Name:      o.serviceNameMapper.MapServiceName(IntegratedServiceName),
		},
	}
	if rErr := o.reconciler.Disable(ctx, k8sConfig, si); rErr != nil {
		return errors.Wrap(rErr, "failed to reconcile the integrated service resource")
	}

	return nil
}

func (o Operator) Apply(ctx context.Context, clusterID uint, spec integratedservices.IntegratedServiceSpec) error {
	ctx, err := o.ensureOrgIDInContext(ctx, clusterID)
	if err != nil {
		return err
	}

	if err := o.clusterService.CheckClusterReady(ctx, clusterID); err != nil {
		return err
	}

	boundSpec, err := bindIntegratedServiceSpec(spec)
	if err != nil {
		return errors.WrapIf(err, "failed to bind integrated service spec")
	}

	if err := boundSpec.Validate(); err != nil {
		return errors.WrapIf(err, "spec validation failed")
	}

	if boundSpec.ExternalDNS.Provider.Name == dnsBanzai {
		if err := o.orgDomainService.EnsureOrgDomain(ctx, clusterID); err != nil {
			return errors.WrapIf(err, "failed to ensure org domain")
		}
	}

	chartValues, err := o.getChartValues(ctx, clusterID, boundSpec)
	if err != nil {
		return errors.WrapIf(err, "failed to get chart values")
	}

	cl, err := o.clusterGetter.GetClusterByIDOnly(ctx, clusterID)
	if err != nil {
		return errors.WrapIf(err, "failed to retrieve the cluster")
	}

	k8sConfig, err := cl.GetK8sConfig()
	if err != nil {
		return errors.WrapIf(err, "failed to retrieve the k8s config")
	}

	si := v1alpha1.ServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: o.config.Namespace,
			Name:      o.serviceNameMapper.MapServiceName(IntegratedServiceName),
		},
		Spec: v1alpha1.ServiceInstanceSpec{
			Service: o.serviceNameMapper.MapServiceName(IntegratedServiceName),
			Config:  string(chartValues),
		},
	}

	if rErr := o.reconciler.Reconcile(ctx, k8sConfig, si); rErr != nil {
		return errors.Wrap(rErr, "failed to reconcile the integrated service resource")
	}

	return nil
}

func (o Operator) Name() string {
	return IntegratedServiceName
}

func (o Operator) getChartValues(ctx context.Context, clusterID uint, spec dnsIntegratedServiceSpec) ([]byte, error) {
	cl, err := o.clusterGetter.GetClusterByIDOnly(ctx, clusterID)
	if err != nil {
		return nil, errors.WrapIf(err, "failed to get cluster")
	}

	chartValues := externaldns.ChartValues{
		Sources: spec.ExternalDNS.Sources,
		RBAC: &externaldns.RBACSettings{
			Create: cl.RbacEnabled(),
		},
		Image: &externaldns.ImageSettings{
			Repository: o.config.Charts.ExternalDNS.Values.Image.Repository,
			Tag:        o.config.Charts.ExternalDNS.Values.Image.Tag,
		},
		DomainFilters: spec.ExternalDNS.DomainFilters,
		Policy:        string(spec.ExternalDNS.Policy),
		TXTOwnerID:    string(spec.ExternalDNS.TXTOwnerID),
		TXTPrefix:     string(spec.ExternalDNS.TXTPrefix),
		Provider:      getProviderNameForChart(spec.ExternalDNS.Provider.Name),
	}

	if spec.ExternalDNS.Provider.Name == dnsBanzai {
		spec.ExternalDNS.Provider.SecretID = route53.IAMUserAccessKeySecretID
	}

	secretValues, err := o.secretStore.GetSecretValues(ctx, spec.ExternalDNS.Provider.SecretID)
	if err != nil {
		return nil, errors.WrapIf(err, "failed to get secret")
	}

	switch spec.ExternalDNS.Provider.Name {
	case dnsBanzai, dnsRoute53:
		chartValues.AWS = &externaldns.AWSSettings{
			Region: secretValues[secrettype.AwsRegion],
			Credentials: &externaldns.AWSCredentials{
				AccessKey: secretValues[secrettype.AwsAccessKeyId],
				SecretKey: secretValues[secrettype.AwsSecretAccessKey],
			},
		}

		if options := spec.ExternalDNS.Provider.Options; options != nil {
			chartValues.AWS.BatchChangeSize = options.BatchChangeSize
			chartValues.AWS.Region = options.Region
		}

	case dnsAzure:
		type azureSecret struct {
			ClientID       string `json:"aadClientId" mapstructure:"AZURE_CLIENT_ID"`
			ClientSecret   string `json:"aadClientSecret" mapstructure:"AZURE_CLIENT_SECRET"`
			TenantID       string `json:"tenantId" mapstructure:"AZURE_TENANT_ID"`
			SubscriptionID string `json:"subscriptionId" mapstructure:"AZURE_SUBSCRIPTION_ID"`
		}

		var secret azureSecret
		if err := mapstructure.Decode(secretValues, &secret); err != nil {
			return nil, errors.WrapIf(err, "failed to decode secret values")
		}

		secretName, err := installSecret(cl, o.config.Namespace, externaldns.AzureSecretName, externaldns.AzureSecretDataKey, secret)
		if err != nil {
			return nil, errors.WrapIfWithDetails(err, "failed to install secret to cluster", "clusterId", clusterID)
		}

		chartValues.Azure = &externaldns.AzureSettings{
			SecretName:    secretName,
			ResourceGroup: spec.ExternalDNS.Provider.Options.AzureResourceGroup,
		}

	case dnsGoogle:
		secretName, err := installSecret(cl, o.config.Namespace, externaldns.GoogleSecretName, externaldns.GoogleSecretDataKey, secretValues)
		if err != nil {
			return nil, errors.WrapIfWithDetails(err, "failed to install secret to cluster", "clusterId", clusterID)
		}

		chartValues.Google = &externaldns.GoogleSettings{
			Project:              secretValues[secrettype.ProjectId],
			ServiceAccountSecret: secretName,
		}

		if options := spec.ExternalDNS.Provider.Options; options != nil {
			chartValues.Google.Project = options.GoogleProject
		}

	default:
	}

	// wrap the original specification into the values byte slice (temporary solution for backwards compatibility)
	var origSpec integratedservices.IntegratedServiceSpec
	if err := mapstructure.Decode(spec, &origSpec); err != nil {
		return nil, errors.WrapIf(err, "failed to decode originalspec ")
	}

	rawValues, err := o.specWrapper.Wrap(ctx, chartValues, origSpec)
	if err != nil {
		return nil, errors.WrapIf(err, "failed to marshal chart values")
	}

	return rawValues, nil
}

func (o Operator) ensureOrgIDInContext(ctx context.Context, clusterID uint) (context.Context, error) {
	if _, ok := auth.GetCurrentOrganizationID(ctx); !ok {
		cluster, err := o.clusterGetter.GetClusterByIDOnly(ctx, clusterID)
		if err != nil {
			return ctx, errors.WrapIf(err, "failed to get cluster by ID")
		}
		ctx = auth.SetCurrentOrganizationID(ctx, cluster.GetOrganizationId())
	}
	return ctx, nil
}
