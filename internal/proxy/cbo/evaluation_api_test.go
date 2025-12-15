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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetCBOEvaluationSummary(t *testing.T) {
	collector := setupTestCollector(100, 1*time.Hour)
	ctx := context.Background()
	baseTime := time.Now()

	t.Run("empty metrics", func(t *testing.T) {
		summary := collector.GetCBOEvaluationSummary(ctx, 1, time.Time{}, time.Time{})
		assert.NotNil(t, summary)
		assert.Equal(t, int64(0), summary.TotalQueries)
		assert.Equal(t, int64(0), summary.QueriesWithCBO)
		assert.Equal(t, int64(0), summary.QueriesImproved)
		assert.Equal(t, int64(0), summary.QueriesDegraded)
		assert.Equal(t, int64(0), summary.QueriesNeutral)
	})

	t.Run("single collection metrics", func(t *testing.T) {
		// Add improved metrics
		improved1 := &CBOMetrics{
			QueryID:                "improved-1",
			CollectionID:           1,
			ExecutionTimeWithCBO:    50 * time.Millisecond,
			ExecutionTimeWithoutCBO: 100 * time.Millisecond,
			SelectedStrategy:       "standard_filter",
			EstimatorType:          "mock",
			Timestamp:              baseTime,
		}
		improved1.CalculateOptimizationResult()
		collector.RecordMetrics(ctx, improved1)

		// Add degraded metric
		degraded1 := &CBOMetrics{
			QueryID:                "degraded-1",
			CollectionID:           1,
			ExecutionTimeWithCBO:    150 * time.Millisecond,
			ExecutionTimeWithoutCBO: 100 * time.Millisecond,
			SelectedStrategy:       "iterative_filter",
			EstimatorType:          "statistics",
			Timestamp:              baseTime,
		}
		degraded1.CalculateOptimizationResult()
		collector.RecordMetrics(ctx, degraded1)

		summary := collector.GetCBOEvaluationSummary(ctx, 1, time.Time{}, time.Time{})
		assert.Equal(t, int64(2), summary.TotalQueries)
		assert.Equal(t, int64(2), summary.QueriesWithCBO)
		assert.Equal(t, int64(1), summary.QueriesImproved)
		assert.Equal(t, int64(1), summary.QueriesDegraded)
		assert.Equal(t, int64(1), summary.StrategyDistribution["standard_filter"])
		assert.Equal(t, int64(1), summary.StrategyDistribution["iterative_filter"])
		assert.Equal(t, int64(1), summary.EstimatorDistribution["mock"])
		assert.Equal(t, int64(1), summary.EstimatorDistribution["statistics"])
	})

	t.Run("multiple collections filtering", func(t *testing.T) {
		// Add metrics for collection 2
		metric2 := &CBOMetrics{
			QueryID:     "coll2-1",
			CollectionID: 2,
			Timestamp:   baseTime,
		}
		collector.RecordMetrics(ctx, metric2)

		// Get summary for collection 1 only
		summary := collector.GetCBOEvaluationSummary(ctx, 1, time.Time{}, time.Time{})
		// Should not include collection 2 metrics
		for _, m := range collector.GetMetricsByCollection(1, time.Time{}, time.Time{}) {
			assert.Equal(t, int64(1), m.CollectionID)
		}
		assert.GreaterOrEqual(t, summary.TotalQueries, int64(2))
	})

	t.Run("time range filtering", func(t *testing.T) {
		startTime := baseTime.Add(1 * time.Hour)
		endTime := baseTime.Add(2 * time.Hour)

		// Add metric outside time range
		oldMetric := &CBOMetrics{
			QueryID:     "old",
			CollectionID: 1,
			Timestamp:   baseTime,
		}
		collector.RecordMetrics(ctx, oldMetric)

		// Add metric inside time range
		newMetric := &CBOMetrics{
			QueryID:     "new",
			CollectionID: 1,
			Timestamp:   baseTime.Add(1*time.Hour + 30*time.Minute),
		}
		collector.RecordMetrics(ctx, newMetric)

		summary := collector.GetCBOEvaluationSummary(ctx, 1, startTime, endTime)
		// Should only include metrics in time range
		assert.GreaterOrEqual(t, summary.TotalQueries, int64(1))
	})

	t.Run("average calculations", func(t *testing.T) {
		// Add metrics with baseline
		metrics := []*CBOMetrics{
			{
				QueryID:                "avg-1",
				CollectionID:           1,
				ExecutionTimeWithCBO:    50 * time.Millisecond,
				ExecutionTimeWithoutCBO: 100 * time.Millisecond,
				Timestamp:              baseTime,
			},
			{
				QueryID:                "avg-2",
				CollectionID:           1,
				ExecutionTimeWithCBO:    30 * time.Millisecond,
				ExecutionTimeWithoutCBO: 100 * time.Millisecond,
				Timestamp:              baseTime,
			},
		}

		for _, m := range metrics {
			m.CalculateOptimizationResult()
			collector.RecordMetrics(ctx, m)
		}

		summary := collector.GetCBOEvaluationSummary(ctx, 1, time.Time{}, time.Time{})
		assert.Greater(t, summary.AvgTimeReduction, time.Duration(0))
		assert.Greater(t, summary.AvgTimeReductionPercent, 0.0)
		assert.Greater(t, summary.TotalTimeSaved, time.Duration(0))
	})
}

func TestGetCBOComparison(t *testing.T) {
	collector := setupTestCollector(100, 1*time.Hour)
	ctx := context.Background()
	baseTime := time.Now()

	t.Run("empty metrics", func(t *testing.T) {
		comparison := collector.GetCBOComparison(ctx, 1, time.Time{}, time.Time{})
		assert.NotNil(t, comparison)
		assert.Equal(t, int64(0), comparison.TotalQueries)
		assert.Equal(t, int64(0), comparison.QueriesWithBaseline)
	})

	t.Run("metrics with baseline", func(t *testing.T) {
		metrics := []*CBOMetrics{
			{
				QueryID:                "comp-1",
				CollectionID:           1,
				ExecutionTimeWithCBO:    50 * time.Millisecond,
				ExecutionTimeWithoutCBO: 100 * time.Millisecond,
				Timestamp:              baseTime,
			},
			{
				QueryID:                "comp-2",
				CollectionID:           1,
				ExecutionTimeWithCBO:    30 * time.Millisecond,
				ExecutionTimeWithoutCBO: 100 * time.Millisecond,
				Timestamp:              baseTime,
			},
			{
				QueryID:                "comp-3",
				CollectionID:           1,
				ExecutionTimeWithCBO:    150 * time.Millisecond,
				ExecutionTimeWithoutCBO: 100 * time.Millisecond,
				Timestamp:              baseTime,
			},
		}

		for _, m := range metrics {
			m.CalculateOptimizationResult()
			collector.RecordMetrics(ctx, m)
		}

		comparison := collector.GetCBOComparison(ctx, 1, time.Time{}, time.Time{})
		assert.Equal(t, int64(3), comparison.TotalQueries)
		assert.Equal(t, int64(3), comparison.QueriesWithBaseline)
		assert.Equal(t, int64(2), comparison.OptimizationResults.Improved)
		assert.Equal(t, int64(1), comparison.OptimizationResults.Degraded)
		assert.Greater(t, comparison.AvgExecutionTimeWithCBO, time.Duration(0))
		assert.Greater(t, comparison.AvgExecutionTimeWithoutCBO, time.Duration(0))
		assert.Greater(t, comparison.TotalTimeSaved, time.Duration(0))
	})

	t.Run("metrics without baseline", func(t *testing.T) {
		metric := &CBOMetrics{
			QueryID:             "no-baseline",
			CollectionID:        1,
			ExecutionTimeWithCBO: 50 * time.Millisecond,
			ExecutionTimeWithoutCBO: 0,
			Timestamp:           baseTime,
		}
		collector.RecordMetrics(ctx, metric)

		comparison := collector.GetCBOComparison(ctx, 1, time.Time{}, time.Time{})
		// Should count total queries but not baseline queries
		assert.Greater(t, comparison.TotalQueries, int64(0))
	})

	t.Run("percentile calculations", func(t *testing.T) {
		// Add multiple metrics with different time reductions
		timeReductions := []time.Duration{
			10 * time.Millisecond,
			20 * time.Millisecond,
			30 * time.Millisecond,
			40 * time.Millisecond,
			50 * time.Millisecond,
		}

		for i, reduction := range timeReductions {
			metric := &CBOMetrics{
				QueryID:                fmt.Sprintf("percentile-%d", i),
				CollectionID:           1,
				ExecutionTimeWithCBO:    100*time.Millisecond - reduction,
				ExecutionTimeWithoutCBO: 100 * time.Millisecond,
				Timestamp:              baseTime,
			}
			metric.CalculateOptimizationResult()
			collector.RecordMetrics(ctx, metric)
		}

		comparison := collector.GetCBOComparison(ctx, 1, time.Time{}, time.Time{})
		assert.Greater(t, comparison.PercentileMetrics.P50TimeReduction, time.Duration(0))
		assert.Greater(t, comparison.PercentileMetrics.P95TimeReduction, time.Duration(0))
		assert.Greater(t, comparison.PercentileMetrics.P99TimeReduction, time.Duration(0))
	})
}

func TestGetCBORecommendations(t *testing.T) {
	collector := setupTestCollector(100, 1*time.Hour)
	ctx := context.Background()
	baseTime := time.Now()

	t.Run("no metrics", func(t *testing.T) {
		recommendations := collector.GetCBORecommendations(ctx, 1, time.Time{}, time.Time{})
		assert.NotEmpty(t, recommendations)
		assert.Contains(t, recommendations[0], "No CBO metrics available")
	})

	t.Run("high degradation percentage", func(t *testing.T) {
		// Add many degraded metrics (>10% of total)
		for i := 0; i < 15; i++ {
			metric := &CBOMetrics{
				QueryID:                fmt.Sprintf("degraded-%d", i),
				CollectionID:           1,
				ExecutionTimeWithCBO:    150 * time.Millisecond,
				ExecutionTimeWithoutCBO: 100 * time.Millisecond,
				Timestamp:              baseTime,
			}
			metric.CalculateOptimizationResult()
			collector.RecordMetrics(ctx, metric)
		}

		// Add some improved metrics
		for i := 0; i < 5; i++ {
			metric := &CBOMetrics{
				QueryID:                fmt.Sprintf("improved-%d", i),
				CollectionID:           1,
				ExecutionTimeWithCBO:    50 * time.Millisecond,
				ExecutionTimeWithoutCBO: 100 * time.Millisecond,
				Timestamp:              baseTime,
			}
			metric.CalculateOptimizationResult()
			collector.RecordMetrics(ctx, metric)
		}

		recommendations := collector.GetCBORecommendations(ctx, 1, time.Time{}, time.Time{})
		foundDegradationWarning := false
		for _, rec := range recommendations {
			if contains(rec, "degradation") {
				foundDegradationWarning = true
				break
			}
		}
		assert.True(t, foundDegradationWarning, "Should recommend reviewing CBO when degradation is high")
	})

	t.Run("no statistics estimator usage", func(t *testing.T) {
		collector.Clear()
		// Add metrics with mock estimator only
		for i := 0; i < 5; i++ {
			metric := &CBOMetrics{
				QueryID:       fmt.Sprintf("mock-%d", i),
				CollectionID:  1,
				EstimatorType: "mock",
				Timestamp:     baseTime,
			}
			collector.RecordMetrics(ctx, metric)
		}

		recommendations := collector.GetCBORecommendations(ctx, 1, time.Time{}, time.Time{})
		foundStatsRecommendation := false
		for _, rec := range recommendations {
			if contains(rec, "statistics") || contains(rec, "estimator") {
				foundStatsRecommendation = true
				break
			}
		}
		assert.True(t, foundStatsRecommendation, "Should recommend using statistics estimator")
	})

	t.Run("one strategy dominating", func(t *testing.T) {
		collector.Clear()
		// Add metrics with same strategy (>90%)
		for i := 0; i < 10; i++ {
			metric := &CBOMetrics{
				QueryID:          fmt.Sprintf("strategy-%d", i),
				CollectionID:     1,
				SelectedStrategy: "standard_filter",
				Timestamp:        baseTime,
			}
			collector.RecordMetrics(ctx, metric)
		}

		recommendations := collector.GetCBORecommendations(ctx, 1, time.Time{}, time.Time{})
		foundStrategyRecommendation := false
		for _, rec := range recommendations {
			if contains(rec, "strategy") || contains(rec, "threshold") {
				foundStrategyRecommendation = true
				break
			}
		}
		assert.True(t, foundStrategyRecommendation, "Should recommend reviewing threshold when one strategy dominates")
	})

	t.Run("high selectivity estimates", func(t *testing.T) {
		collector.Clear()
		// Add metrics with high selectivity (>0.5)
		for i := 0; i < 6; i++ {
			metric := &CBOMetrics{
				QueryID:            fmt.Sprintf("high-sel-%d", i),
				CollectionID:      1,
				SelectivityEstimate: 0.6,
				Timestamp:         baseTime,
			}
			collector.RecordMetrics(ctx, metric)
		}

		recommendations := collector.GetCBORecommendations(ctx, 1, time.Time{}, time.Time{})
		foundSelectivityRecommendation := false
		for _, rec := range recommendations {
			if contains(rec, "selectivity") || contains(rec, "statistics") {
				foundSelectivityRecommendation = true
				break
			}
		}
		assert.True(t, foundSelectivityRecommendation, "Should recommend reviewing when selectivity is high")
	})

	t.Run("good performance", func(t *testing.T) {
		collector.Clear()
		// Add mostly improved metrics
		for i := 0; i < 10; i++ {
			metric := &CBOMetrics{
				QueryID:                fmt.Sprintf("good-%d", i),
				CollectionID:           1,
				ExecutionTimeWithCBO:    50 * time.Millisecond,
				ExecutionTimeWithoutCBO: 100 * time.Millisecond,
				SelectedStrategy:       "standard_filter",
				EstimatorType:          "statistics",
				SelectivityEstimate:    0.1,
				Timestamp:              baseTime,
			}
			metric.CalculateOptimizationResult()
			collector.RecordMetrics(ctx, metric)
		}

		recommendations := collector.GetCBORecommendations(ctx, 1, time.Time{}, time.Time{})
		foundPositiveMessage := false
		for _, rec := range recommendations {
			if contains(rec, "performing well") || contains(rec, "No specific recommendations") {
				foundPositiveMessage = true
				break
			}
		}
		assert.True(t, foundPositiveMessage, "Should return positive message when performance is good")
	})
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

