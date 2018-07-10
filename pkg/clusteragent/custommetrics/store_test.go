// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubeapiserver

package custommetrics

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewConfigMapStore(t *testing.T) {
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
		},
	}

	client := fake.NewSimpleClientset()
	_, err := client.CoreV1().ConfigMaps("default").Create(cm)
	require.NoError(t, err)

	// configmap already exists
	store, err := NewConfigMapStore(client, "default", "foo")
	require.NoError(t, err)
	require.NotNil(t, store.(*configMapStore).cm)

	// configmap doesn't exist
	store, err = NewConfigMapStore(client, "default", "bar")
	require.NoError(t, err)
	require.NotNil(t, store.(*configMapStore).cm)
}

func TestConfigMapStoreExternalMetrics(t *testing.T) {
	client := fake.NewSimpleClientset()

	tests := []struct {
		desc     string
		metrics  []ExternalMetricValue
		expected []ExternalMetricValue
	}{
		{
			"same metric with different hpas and labels",
			[]ExternalMetricValue{
				{
					MetricName:   "requests_per_s",
					Labels:       map[string]string{"role": "frontend"},
					HPAName:      "foo",
					HPANamespace: "default",
				},
				{
					MetricName:   "requests_per_s",
					Labels:       map[string]string{"role": "backend"},
					HPAName:      "bar",
					HPANamespace: "default",
				},
			},
			[]ExternalMetricValue{
				{
					MetricName:   "requests_per_s",
					Labels:       map[string]string{"role": "frontend"},
					HPAName:      "foo",
					HPANamespace: "default",
				},
				{
					MetricName:   "requests_per_s",
					Labels:       map[string]string{"role": "backend"},
					HPAName:      "bar",
					HPANamespace: "default",
				},
			},
		},
		{
			"same metric with different owners and same labels",
			[]ExternalMetricValue{
				{
					MetricName:   "requests_per_s",
					Labels:       map[string]string{"role": "frontend"},
					HPAName:      "foo",
					HPANamespace: "default",
				},
				{
					MetricName:   "requests_per_s",
					Labels:       map[string]string{"role": "frontend"},
					HPAName:      "bar",
					HPANamespace: "default",
				},
			},
			[]ExternalMetricValue{
				{
					MetricName:   "requests_per_s",
					Labels:       map[string]string{"role": "frontend"},
					HPAName:      "foo",
					HPANamespace: "default",
				},
				{
					MetricName:   "requests_per_s",
					Labels:       map[string]string{"role": "frontend"},
					HPAName:      "bar",
					HPANamespace: "default",
				},
			},
		},
		{
			"same metric with same owners and different labels",
			[]ExternalMetricValue{
				{
					MetricName:   "requests_per_s",
					Labels:       map[string]string{"role": "frontend"},
					HPAName:      "foo",
					HPANamespace: "default",
				},
				{
					MetricName:   "requests_per_s",
					Labels:       map[string]string{"role": "backend"},
					HPAName:      "foo",
					HPANamespace: "default",
				},
			},
			[]ExternalMetricValue{
				{
					MetricName:   "requests_per_s",
					Labels:       map[string]string{"role": "backend"},
					HPAName:      "foo",
					HPANamespace: "default",
				},
			},
		},
	}

	for i, tt := range tests {
		t.Run(fmt.Sprintf("#%d %s", i, tt.desc), func(t *testing.T) {
			store, err := NewConfigMapStore(client, "default", fmt.Sprintf("test-%d", i))
			require.NoError(t, err)
			require.NotNil(t, store.(*configMapStore).cm)

			err = store.SetExternalMetrics(tt.metrics)
			require.NoError(t, err)

			allMetrics, err := store.ListAllExternalMetrics()
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.expected, allMetrics)

			infos := make([]ExternalMetricInfo, 0)
			for _, m := range tt.metrics {
				info := ExternalMetricInfo{
					MetricName:   m.MetricName,
					HPAName:      m.HPAName,
					HPANamespace: m.HPANamespace,
				}
				infos = append(infos, info)
			}

			err = store.DeleteExternalMetrics(infos)
			require.NoError(t, err)
			assert.Zero(t, len(store.(*configMapStore).cm.Data))
		})
	}
}
