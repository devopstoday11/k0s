/*
Copyright 2024 k0s authors

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

package controller_test

import (
	"context"
	"errors"
	"os"
	"testing"

	internallog "github.com/k0sproject/k0s/internal/pkg/log"
	"github.com/k0sproject/k0s/internal/testutil"
	k0sv1beta1 "github.com/k0sproject/k0s/pkg/apis/k0s/v1beta1"
	"github.com/k0sproject/k0s/pkg/component/controller"
	"github.com/k0sproject/k0s/pkg/component/controller/leaderelector"
	"github.com/k0sproject/k0s/pkg/constant"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClusterConfigInitializer_Create(t *testing.T) {
	clients := testutil.NewFakeClientFactory()
	leaderElector := leaderelector.Dummy{Leader: true}
	initialConfig := k0sv1beta1.DefaultClusterConfig()
	initialConfig.ResourceVersion = "42"

	underTest, err := controller.NewClusterConfigInitializer(
		clients, &leaderElector, initialConfig.DeepCopy(),
	)
	require.NoError(t, err)

	require.NoError(t, underTest.Init(context.TODO()))
	require.NoError(t, underTest.Start(context.TODO()))
	t.Cleanup(func() { assert.NoError(t, underTest.Stop()) })

	actualConfig, err := clients.K0sClient.K0sV1beta1().
		ClusterConfigs(constant.ClusterConfigNamespace).
		Get(context.TODO(), "k0s", metav1.GetOptions{})
	require.NoError(t, err)

	assert.Equal(t, initialConfig, actualConfig)
}

func TestClusterConfigInitializer_NoConfig(t *testing.T) {
	clients := testutil.NewFakeClientFactory()
	leaderElector := leaderelector.Dummy{Leader: false}
	initialConfig := k0sv1beta1.DefaultClusterConfig()

	underTest, err := controller.NewClusterConfigInitializer(
		clients, &leaderElector, initialConfig.DeepCopy(),
	)
	require.NoError(t, err)

	ctx, cancel := context.WithCancelCause(context.TODO())
	t.Cleanup(func() { cancel(nil) })

	var gets uint
	abortTest := errors.New("aborting test after some retries")
	clients.K0sClient.PrependReactor("get", "clusterconfigs", func(action k8stesting.Action) (bool, runtime.Object, error) {
		gets++ // Let's observe some retries, then cancel the context.
		if gets > 1 {
			cancel(abortTest)
		}
		return false, nil, nil
	})

	require.NoError(t, underTest.Init(ctx))

	err = underTest.Start(ctx)
	assert.ErrorContains(t, err, "failed to ensure the existence of the cluster configuration: ")
	assert.ErrorIs(t, err, context.Canceled)
}

func TestClusterConfigInitializer_Exists(t *testing.T) {
	test := func(t *testing.T, leader bool) {
		existingConfig := k0sv1beta1.DefaultClusterConfig()
		existingConfig.ResourceVersion = "42"
		clients := testutil.NewFakeClientFactory(existingConfig)
		leaderElector := leaderelector.Dummy{Leader: leader}
		initialConfig := existingConfig.DeepCopy()
		initialConfig.ResourceVersion = "1337"

		underTest, err := controller.NewClusterConfigInitializer(
			clients, &leaderElector, initialConfig,
		)
		require.NoError(t, err)

		require.NoError(t, underTest.Init(context.TODO()))
		require.NoError(t, underTest.Start(context.TODO()))
		t.Cleanup(func() { assert.NoError(t, underTest.Stop()) })

		actualConfig, err := clients.K0sClient.K0sV1beta1().
			ClusterConfigs(constant.ClusterConfigNamespace).
			Get(context.TODO(), "k0s", metav1.GetOptions{})
		require.NoError(t, err)

		assert.Equal(t, existingConfig, actualConfig)
	}

	t.Run("Leader", func(t *testing.T) { test(t, true) })
	t.Run("Follower", func(t *testing.T) { test(t, false) })
}

func TestMain(m *testing.M) {
	internallog.SetDebugLevel()
	os.Exit(m.Run())
}
