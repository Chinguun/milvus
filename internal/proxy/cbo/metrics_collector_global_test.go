// Licensed to the LF AI & Data foundation under one
// or more contributor license agreements. See the NOTICE file
// distributed with this work for additional information
// regarding copyright ownership. The ASF licenses this file
// to you under the Apache License, Version 2.0 (the
// "License"); you may not use this file except in compliance
// with the License. You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cbo

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/milvus-io/milvus/pkg/v2/util/paramtable"
)

func TestGetGlobalCBOMetricsCollector_Enabled(t *testing.T) {
	// Reset global state
	globalCBOMetricsCollector = nil
	globalCBOMetricsCollectorOnce = sync.Once{}

	// Set config to enabled
	paramtable.Get().ProxyCfg.CBOEvaluationEnabled = paramtable.NewParamItem("proxy.cbo.evaluation.enabled", "true")
	paramtable.Get().ProxyCfg.CBOEvaluationMaxMetrics = paramtable.NewParamItem("proxy.cbo.evaluation.maxMetrics", "1000")
	paramtable.Get().ProxyCfg.CBOEvaluationRetention = paramtable.NewParamItem("proxy.cbo.evaluation.retention", "1h")

	collector1 := GetGlobalCBOMetricsCollector()
	assert.NotNil(t, collector1)

	// Verify singleton pattern - multiple calls return same instance
	collector2 := GetGlobalCBOMetricsCollector()
	assert.Equal(t, collector1, collector2)

	// Verify collector is functional
	stats := collector1.GetStats()
	assert.Equal(t, 1000, stats["max_size"])
	assert.Equal(t, "1h0m0s", stats["retention"])

	// Test that it can record metrics
	ctx := context.Background()
	metric := &CBOMetrics{
		QueryID:     "test-query",
		CollectionID: 1,
		Timestamp:   time.Now(),
	}
	collector1.RecordMetrics(ctx, metric)

	retrieved, exists := collector1.GetMetrics("test-query")
	assert.True(t, exists)
	assert.Equal(t, "test-query", retrieved.QueryID)
}

func TestGetGlobalCBOMetricsCollector_Disabled(t *testing.T) {
	// Reset global state
	globalCBOMetricsCollector = nil
	globalCBOMetricsCollectorOnce = sync.Once{}

	// Set config to disabled
	paramtable.Get().ProxyCfg.CBOEvaluationEnabled = paramtable.NewParamItem("proxy.cbo.evaluation.enabled", "false")
	paramtable.Get().ProxyCfg.CBOEvaluationMaxMetrics = paramtable.NewParamItem("proxy.cbo.evaluation.maxMetrics", "1000")
	paramtable.Get().ProxyCfg.CBOEvaluationRetention = paramtable.NewParamItem("proxy.cbo.evaluation.retention", "1h")

	collector := GetGlobalCBOMetricsCollector()
	assert.NotNil(t, collector)

	// Verify it's created with maxSize=0, retention=0
	stats := collector.GetStats()
	assert.Equal(t, 0, stats["max_size"])
	assert.Equal(t, "0s", stats["retention"])

	// Verify it still works (doesn't crash) but doesn't store metrics
	ctx := context.Background()
	metric := &CBOMetrics{
		QueryID:     "test-query-disabled",
		CollectionID: 1,
		Timestamp:   time.Now(),
	}

	// Should not panic
	assert.NotPanics(t, func() {
		collector.RecordMetrics(ctx, metric)
	})

	// Metrics might not be stored due to maxSize=0, but collector should not crash
	stats = collector.GetStats()
	assert.NotNil(t, stats)
}

func TestGetGlobalCBOMetricsCollector_Singleton(t *testing.T) {
	// Reset global state
	globalCBOMetricsCollector = nil
	globalCBOMetricsCollectorOnce = sync.Once{}

	// Set config
	paramtable.Get().ProxyCfg.CBOEvaluationEnabled = paramtable.NewParamItem("proxy.cbo.evaluation.enabled", "true")
	paramtable.Get().ProxyCfg.CBOEvaluationMaxMetrics = paramtable.NewParamItem("proxy.cbo.evaluation.maxMetrics", "500")
	paramtable.Get().ProxyCfg.CBOEvaluationRetention = paramtable.NewParamItem("proxy.cbo.evaluation.retention", "2h")

	// Call multiple times concurrently
	var collectors []*CBOMetricsCollector
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			collector := GetGlobalCBOMetricsCollector()
			mu.Lock()
			collectors = append(collectors, collector)
			mu.Unlock()
		}()
	}

	wg.Wait()

	// All should be the same instance
	firstCollector := collectors[0]
	for _, c := range collectors {
		assert.Equal(t, firstCollector, c, "All calls should return the same singleton instance")
	}
}

