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
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func createTestMetrics(collectionID int64, numMetrics int, baseTime time.Time) []*CBOMetrics {
	metrics := make([]*CBOMetrics, numMetrics)
	for i := 0; i < numMetrics; i++ {
		metrics[i] = &CBOMetrics{
			QueryID:            fmt.Sprintf("query-%d", i),
			CollectionID:       collectionID,
			CollectionName:     "test_collection",
			FilterExpression:   "price < 10",
			Timestamp:          baseTime.Add(time.Duration(i) * time.Second),
			SelectivityEstimate: 0.1,
			SelectedStrategy:   "standard_filter",
			EstimatorType:      "mock",
			ExecutionTimeWithCBO: 50 * time.Millisecond,
			NumQueries:         1,
			TopK:               10,
		}
	}
	return metrics
}

func createTestMetricsWithBaseline(collectionID int64, numMetrics int, baseTime time.Time) []*CBOMetrics {
	metrics := createTestMetrics(collectionID, numMetrics, baseTime)
	for i := 0; i < numMetrics; i++ {
		metrics[i].ExecutionTimeWithoutCBO = 100 * time.Millisecond
		metrics[i].CalculateOptimizationResult()
	}
	return metrics
}

func setupTestCollector(maxSize int, retention time.Duration) *CBOMetricsCollector {
	return NewCBOMetricsCollector(maxSize, retention)
}

func TestNewCBOMetricsCollector(t *testing.T) {
	t.Run("with maxSize and retention", func(t *testing.T) {
		collector := NewCBOMetricsCollector(100, 1*time.Hour)
		assert.NotNil(t, collector)
		stats := collector.GetStats()
		assert.Equal(t, 100, stats["max_size"])
		assert.Equal(t, "1h0m0s", stats["retention"])
	})

	t.Run("with zero maxSize", func(t *testing.T) {
		collector := NewCBOMetricsCollector(0, 1*time.Hour)
		assert.NotNil(t, collector)
	})

	t.Run("with zero retention", func(t *testing.T) {
		collector := NewCBOMetricsCollector(100, 0)
		assert.NotNil(t, collector)
		stats := collector.GetStats()
		assert.Equal(t, "0s", stats["retention"])
	})
}

func TestRecordMetrics(t *testing.T) {
	collector := setupTestCollector(100, 1*time.Hour)
	ctx := context.Background()

	t.Run("record single metric", func(t *testing.T) {
		metric := &CBOMetrics{
			QueryID:        "query-1",
			CollectionID:   1,
			CollectionName: "test_collection",
			Timestamp:      time.Now(),
		}
		collector.RecordMetrics(ctx, metric)

		retrieved, exists := collector.GetMetrics("query-1")
		assert.True(t, exists)
		assert.Equal(t, "query-1", retrieved.QueryID)
		assert.Equal(t, int64(1), retrieved.CollectionID)
	})

	t.Run("record multiple metrics", func(t *testing.T) {
		metrics := createTestMetrics(1, 5, time.Now())
		for _, m := range metrics {
			collector.RecordMetrics(ctx, m)
		}

		allMetrics := collector.GetAllMetrics()
		assert.GreaterOrEqual(t, len(allMetrics), 5)
	})

	t.Run("record nil metric", func(t *testing.T) {
		// Should not panic
		assert.NotPanics(t, func() {
			collector.RecordMetrics(ctx, nil)
		})
	})

	t.Run("calculate optimization result automatically", func(t *testing.T) {
		metric := &CBOMetrics{
			QueryID:                "query-opt",
			CollectionID:           1,
			ExecutionTimeWithCBO:    50 * time.Millisecond,
			ExecutionTimeWithoutCBO: 100 * time.Millisecond,
		}
		collector.RecordMetrics(ctx, metric)

		retrieved, exists := collector.GetMetrics("query-opt")
		assert.True(t, exists)
		assert.Equal(t, OptimizationResultImproved, retrieved.OptimizationResult)
		assert.Equal(t, 50.0, retrieved.TimeReductionPercent)
	})
}

func TestGetMetrics(t *testing.T) {
	collector := setupTestCollector(100, 1*time.Hour)
	ctx := context.Background()

	metric := &CBOMetrics{
		QueryID:        "query-1",
		CollectionID:   1,
		CollectionName: "test_collection",
		Timestamp:      time.Now(),
	}
	collector.RecordMetrics(ctx, metric)

	t.Run("get existing metric", func(t *testing.T) {
		retrieved, exists := collector.GetMetrics("query-1")
		assert.True(t, exists)
		assert.Equal(t, "query-1", retrieved.QueryID)
	})

	t.Run("get non-existent metric", func(t *testing.T) {
		_, exists := collector.GetMetrics("non-existent")
		assert.False(t, exists)
	})

	t.Run("returned metric is a copy", func(t *testing.T) {
		retrieved1, _ := collector.GetMetrics("query-1")
		retrieved2, _ := collector.GetMetrics("query-1")

		// Modify one
		retrieved1.CollectionName = "modified"

		// Other should be unchanged
		assert.Equal(t, "test_collection", retrieved2.CollectionName)
	})
}

func TestGetMetricsByCollection(t *testing.T) {
	collector := setupTestCollector(100, 1*time.Hour)
	ctx := context.Background()
	baseTime := time.Now()

	// Add metrics for collection 1
	metrics1 := createTestMetrics(1, 3, baseTime)
	for _, m := range metrics1 {
		collector.RecordMetrics(ctx, m)
	}

	// Add metrics for collection 2
	metrics2 := createTestMetrics(2, 2, baseTime)
	for _, m := range metrics2 {
		collector.RecordMetrics(ctx, m)
	}

	t.Run("get metrics for specific collection", func(t *testing.T) {
		result := collector.GetMetricsByCollection(1, time.Time{}, time.Time{})
		assert.Equal(t, 3, len(result))
		for _, m := range result {
			assert.Equal(t, int64(1), m.CollectionID)
		}
	})

	t.Run("filter by time range", func(t *testing.T) {
		startTime := baseTime.Add(1 * time.Second)
		endTime := baseTime.Add(2 * time.Second)
		result := collector.GetMetricsByCollection(1, startTime, endTime)
		assert.GreaterOrEqual(t, len(result), 1)
		for _, m := range result {
			assert.True(t, !m.Timestamp.Before(startTime) && !m.Timestamp.After(endTime))
		}
	})

	t.Run("empty collection", func(t *testing.T) {
		result := collector.GetMetricsByCollection(999, time.Time{}, time.Time{})
		assert.Equal(t, 0, len(result))
	})
}

func TestGetMetricsByTimeRange(t *testing.T) {
	collector := setupTestCollector(100, 1*time.Hour)
	ctx := context.Background()
	baseTime := time.Now()

	metrics := createTestMetrics(1, 5, baseTime)
	for _, m := range metrics {
		collector.RecordMetrics(ctx, m)
	}

	t.Run("get metrics within time range", func(t *testing.T) {
		startTime := baseTime
		endTime := baseTime.Add(3 * time.Second)
		result := collector.GetMetricsByTimeRange(startTime, endTime)
		assert.GreaterOrEqual(t, len(result), 3)
		for _, m := range result {
			assert.True(t, !m.Timestamp.Before(startTime) && !m.Timestamp.After(endTime))
		}
	})

	t.Run("filter metrics outside time range", func(t *testing.T) {
		startTime := baseTime.Add(10 * time.Second)
		endTime := baseTime.Add(20 * time.Second)
		result := collector.GetMetricsByTimeRange(startTime, endTime)
		assert.Equal(t, 0, len(result))
	})

	t.Run("zero time returns all", func(t *testing.T) {
		result := collector.GetMetricsByTimeRange(time.Time{}, time.Time{})
		assert.GreaterOrEqual(t, len(result), 5)
	})
}

func TestGetAllMetrics(t *testing.T) {
	collector := setupTestCollector(100, 1*time.Hour)
	ctx := context.Background()

	metrics := createTestMetrics(1, 5, time.Now())
	for _, m := range metrics {
		collector.RecordMetrics(ctx, m)
	}

	t.Run("get all stored metrics", func(t *testing.T) {
		allMetrics := collector.GetAllMetrics()
		assert.GreaterOrEqual(t, len(allMetrics), 5)
	})

	t.Run("returned metrics are copies", func(t *testing.T) {
		allMetrics := collector.GetAllMetrics()
		if len(allMetrics) > 0 {
			originalName := allMetrics[0].CollectionName
			allMetrics[0].CollectionName = "modified"

			// Get again, should be unchanged
			allMetrics2 := collector.GetAllMetrics()
			for _, m := range allMetrics2 {
				if m.QueryID == allMetrics[0].QueryID {
					assert.Equal(t, originalName, m.CollectionName)
					break
				}
			}
		}
	})
}

func TestClear(t *testing.T) {
	collector := setupTestCollector(100, 1*time.Hour)
	ctx := context.Background()

	metrics := createTestMetrics(1, 5, time.Now())
	for _, m := range metrics {
		collector.RecordMetrics(ctx, m)
	}

	assert.Greater(t, len(collector.GetAllMetrics()), 0)

	collector.Clear()

	assert.Equal(t, 0, len(collector.GetAllMetrics()))
	stats := collector.GetStats()
	assert.Equal(t, 0, stats["total_metrics"])
}

func TestGetStats(t *testing.T) {
	collector := setupTestCollector(100, 1*time.Hour)
	ctx := context.Background()

	stats := collector.GetStats()
	assert.Equal(t, 0, stats["total_metrics"])
	assert.Equal(t, 100, stats["max_size"])
	assert.Equal(t, "1h0m0s", stats["retention"])
	assert.Equal(t, 0, stats["collections"])

	// Add some metrics
	metrics := createTestMetrics(1, 3, time.Now())
	for _, m := range metrics {
		collector.RecordMetrics(ctx, m)
	}

	stats = collector.GetStats()
	assert.Equal(t, 3, stats["total_metrics"])
	assert.Equal(t, 1, stats["collections"])
}

func TestCleanup_Retention(t *testing.T) {
	collector := setupTestCollector(100, 1*time.Second)
	ctx := context.Background()

	// Add old metric
	oldMetric := &CBOMetrics{
		QueryID:     "old-query",
		CollectionID: 1,
		Timestamp:   time.Now().Add(-2 * time.Second),
	}
	collector.RecordMetrics(ctx, oldMetric)

	// Add new metric
	newMetric := &CBOMetrics{
		QueryID:     "new-query",
		CollectionID: 1,
		Timestamp:   time.Now(),
	}
	collector.RecordMetrics(ctx, newMetric)

	// Trigger cleanup by recording another metric
	// (cleanup is called in RecordMetrics)
	anotherMetric := &CBOMetrics{
		QueryID:     "another-query",
		CollectionID: 1,
		Timestamp:   time.Now(),
	}
	collector.RecordMetrics(ctx, anotherMetric)

	// Old metric should be removed
	_, exists := collector.GetMetrics("old-query")
	assert.False(t, exists)

	// New metric should still exist
	_, exists = collector.GetMetrics("new-query")
	assert.True(t, exists)
}

func TestEviction_MaxSize(t *testing.T) {
	collector := setupTestCollector(3, 1*time.Hour)
	ctx := context.Background()
	baseTime := time.Now()

	// Add metrics up to maxSize
	metrics := createTestMetrics(1, 3, baseTime)
	for i, m := range metrics {
		m.QueryID = fmt.Sprintf("query-%d", i)
		collector.RecordMetrics(ctx, m)
	}

	assert.Equal(t, 3, len(collector.GetAllMetrics()))

		// Add one more - should evict oldest
		oldestQueryID := fmt.Sprintf("query-%d", 0)
		newMetric := &CBOMetrics{
			QueryID:      "query-new",
			CollectionID: 1,
			Timestamp:    baseTime.Add(10 * time.Second),
		}
		collector.RecordMetrics(ctx, newMetric)

		// Oldest should be evicted
		_, exists := collector.GetMetrics(oldestQueryID)
		assert.False(t, exists)

	// New metric should exist
	_, exists = collector.GetMetrics("query-new")
	assert.True(t, exists)

	// Should still have maxSize metrics
	assert.Equal(t, 3, len(collector.GetAllMetrics()))
}

func TestConcurrentAccess(t *testing.T) {
	collector := setupTestCollector(1000, 1*time.Hour)
	ctx := context.Background()

	var wg sync.WaitGroup
	numGoroutines := 10
	metricsPerGoroutine := 10

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < metricsPerGoroutine; j++ {
				metric := &CBOMetrics{
					QueryID:      fmt.Sprintf("query-%d-%d", goroutineID, j),
					CollectionID: int64(goroutineID),
					Timestamp:    time.Now(),
				}
				collector.RecordMetrics(ctx, metric)
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < metricsPerGoroutine; j++ {
				queryID := fmt.Sprintf("query-%d-%d", goroutineID, j)
				collector.GetMetrics(queryID)
			}
		}(i)
	}

	wg.Wait()

	// Verify all metrics were recorded
	allMetrics := collector.GetAllMetrics()
	assert.GreaterOrEqual(t, len(allMetrics), numGoroutines*metricsPerGoroutine)
}

func TestIndexes_Maintained(t *testing.T) {
	collector := setupTestCollector(100, 1*time.Hour)
	ctx := context.Background()
	baseTime := time.Now()

	// Add metrics for different collections
	metrics1 := createTestMetrics(1, 3, baseTime)
	metrics2 := createTestMetrics(2, 2, baseTime)

	for _, m := range metrics1 {
		collector.RecordMetrics(ctx, m)
	}
	for _, m := range metrics2 {
		collector.RecordMetrics(ctx, m)
	}

	// Verify collection index
	result1 := collector.GetMetricsByCollection(1, time.Time{}, time.Time{})
	assert.Equal(t, 3, len(result1))

	result2 := collector.GetMetricsByCollection(2, time.Time{}, time.Time{})
	assert.Equal(t, 2, len(result2))

	// Verify time range index is sorted
	allMetrics := collector.GetMetricsByTimeRange(time.Time{}, time.Time{})
	if len(allMetrics) > 1 {
		for i := 1; i < len(allMetrics); i++ {
			assert.True(t, !allMetrics[i].Timestamp.Before(allMetrics[i-1].Timestamp),
				"Time range index should be sorted")
		}
	}

	// Test index rebuild after cleanup
	collector.Clear()
	oldMetric := &CBOMetrics{
		QueryID:     "old",
		CollectionID: 1,
		Timestamp:   time.Now().Add(-2 * time.Hour),
	}
	collector.RecordMetrics(ctx, oldMetric)

	// Trigger cleanup
	collector.RecordMetrics(ctx, &CBOMetrics{
		QueryID:     "new",
		CollectionID: 1,
		Timestamp:   time.Now(),
	})

	// Indexes should still work
	result := collector.GetMetricsByCollection(1, time.Time{}, time.Time{})
	assert.GreaterOrEqual(t, len(result), 1)
}

func TestDisabledCollector_NoOp(t *testing.T) {
	// Test that disabled collector (maxSize <= 0) is truly no-op
	collector := NewCBOMetricsCollector(0, 0)
	ctx := context.Background()

	t.Run("RecordMetrics does nothing", func(t *testing.T) {
		metric := &CBOMetrics{
			QueryID:      "test-query",
			CollectionID: 1,
			Timestamp:    time.Now(),
		}
		collector.RecordMetrics(ctx, metric)

		// Should not store anything
		_, exists := collector.GetMetrics("test-query")
		assert.False(t, exists)
		assert.Equal(t, 0, len(collector.GetAllMetrics()))
	})

	t.Run("GetMetrics returns false", func(t *testing.T) {
		_, exists := collector.GetMetrics("any-query")
		assert.False(t, exists)
	})

	t.Run("GetMetricsByCollection returns empty", func(t *testing.T) {
		result := collector.GetMetricsByCollection(1, time.Time{}, time.Time{})
		assert.Equal(t, 0, len(result))
	})

	t.Run("GetMetricsByTimeRange returns empty", func(t *testing.T) {
		result := collector.GetMetricsByTimeRange(time.Time{}, time.Time{})
		assert.Equal(t, 0, len(result))
	})

	t.Run("GetAllMetrics returns empty", func(t *testing.T) {
		result := collector.GetAllMetrics()
		assert.Equal(t, 0, len(result))
	})

	t.Run("GetStats returns zeros", func(t *testing.T) {
		stats := collector.GetStats()
		assert.Equal(t, 0, stats["total_metrics"])
		assert.Equal(t, 0, stats["max_size"])
		assert.Equal(t, 0, stats["collections"])
	})
}

func TestSortMetricsByTimestamp(t *testing.T) {
	baseTime := time.Now()
	metrics := []*CBOMetrics{
		{QueryID: "query-1", Timestamp: baseTime.Add(3 * time.Second)},
		{QueryID: "query-2", Timestamp: baseTime.Add(1 * time.Second)},
		{QueryID: "query-3", Timestamp: baseTime.Add(2 * time.Second)},
	}

	t.Run("sort ascending", func(t *testing.T) {
		sorted := make([]*CBOMetrics, len(metrics))
		copy(sorted, metrics)
		SortMetricsByTimestamp(sorted, "asc")

		assert.Equal(t, "query-2", sorted[0].QueryID)
		assert.Equal(t, "query-3", sorted[1].QueryID)
		assert.Equal(t, "query-1", sorted[2].QueryID)
	})

	t.Run("sort descending", func(t *testing.T) {
		sorted := make([]*CBOMetrics, len(metrics))
		copy(sorted, metrics)
		SortMetricsByTimestamp(sorted, "desc")

		assert.Equal(t, "query-1", sorted[0].QueryID)
		assert.Equal(t, "query-3", sorted[1].QueryID)
		assert.Equal(t, "query-2", sorted[2].QueryID)
	})

	t.Run("default to descending", func(t *testing.T) {
		sorted := make([]*CBOMetrics, len(metrics))
		copy(sorted, metrics)
		SortMetricsByTimestamp(sorted, "invalid")

		assert.Equal(t, "query-1", sorted[0].QueryID)
	})
}

func TestPaginateMetrics(t *testing.T) {
	metrics := make([]*CBOMetrics, 10)
	for i := 0; i < 10; i++ {
		metrics[i] = &CBOMetrics{
			QueryID: fmt.Sprintf("query-%d", i),
		}
	}

	t.Run("paginate with valid limit and offset", func(t *testing.T) {
		result, total := PaginateMetrics(metrics, 3, 2)
		assert.Equal(t, 3, len(result))
		assert.Equal(t, 10, total)
		assert.Equal(t, "query-2", result[0].QueryID)
		assert.Equal(t, "query-4", result[2].QueryID)
	})

	t.Run("paginate with offset beyond length", func(t *testing.T) {
		result, total := PaginateMetrics(metrics, 5, 10)
		assert.Equal(t, 0, len(result))
		assert.Equal(t, 10, total)
	})

	t.Run("paginate with limit exceeding length", func(t *testing.T) {
		result, total := PaginateMetrics(metrics, 5, 7)
		assert.Equal(t, 3, len(result))
		assert.Equal(t, 10, total)
		assert.Equal(t, "query-7", result[0].QueryID)
	})

	t.Run("paginate with zero offset", func(t *testing.T) {
		result, total := PaginateMetrics(metrics, 3, 0)
		assert.Equal(t, 3, len(result))
		assert.Equal(t, 10, total)
		assert.Equal(t, "query-0", result[0].QueryID)
	})
}

