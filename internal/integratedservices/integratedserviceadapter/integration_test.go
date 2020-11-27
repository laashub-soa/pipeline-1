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

package integratedserviceadapter

import (
	"context"
	"flag"
	"fmt"
	"regexp"
	"testing"

	"emperror.dev/errors"
	"github.com/banzaicloud/integrated-service-sdk/api/v1alpha1"
	"github.com/banzaicloud/pipeline/internal/integratedservices"
	internaltesting "github.com/banzaicloud/pipeline/internal/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestIntegration(t *testing.T) {
	if m := flag.Lookup("test.run").Value.String(); m == "" || !regexp.MustCompile(m).MatchString(t.Name()) {
		t.Skip("skipping as execution was not requested explicitly using go test -run")
	}

	t.Run("testGetIntegratedService", testGetIntegratedService)
}

func testGetIntegratedService(t *testing.T) {
	kubeConfig := internaltesting.KubeConfigFromEnv(t)

	client, err := clientForConfig(kubeConfig)
	require.NoError(t, err)

	client.Create(context.TODO(), &v1alpha1.ServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "pipeline-system",
			Name:      "test",
		},
	})

	repo := NewCRRepository(kubeConfigFunc(t), &integratedservices.NoopLogger{}, nil, "pipeline-system")

	service, err := repo.GetIntegratedService(context.TODO(), 123, "test")
	assert.NoError(t, err)
	fmt.Println(service)
}

func kubeConfigFunc(t *testing.T) integratedservices.ClusterKubeConfigFunc {
	return func(ctx context.Context, clusterID uint) ([]byte, error) {
		return internaltesting.KubeConfigFromEnv(t), nil
	}
}

func clientForConfig(kubeConfig []byte) (client.Client, error) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)

	restCfg, err := clientcmd.RESTConfigFromKubeConfig(kubeConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create rest config from cluster configuration")
	}

	cli, err := client.New(restCfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create the client from rest configuration")
	}

	return cli, nil
}
