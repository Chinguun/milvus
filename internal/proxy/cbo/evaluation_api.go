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
	"sort"
	"time"

	"go.uber.org/zap"

	"github.com/milvus-io/milvus/pkg/v2/log"
)

// GetCBOEvaluationSummary calculates and returns an evaluation summary for the given criteria
func (c *CBOMetricsCollector) GetCBOEvaluationSummary(
	ctx context.Context,
	collectionID int64,
	startTime, endTime time.Time,
) *CBOEvaluationSummary {
	metrics := c.GetMetricsByCollection(collectionID, startTime, endTime)

	summary := &CBOEvaluationSummary{
		TotalQueries:           0,
		QueriesWithCBO:         0,
		QueriesImproved:        0,
		QueriesDegraded:        0,
		QueriesNeutral:          0,
		AvgTimeReduction:        0,
		AvgTimeReductionPercent: 0,
		TotalTimeSaved:          0,
		StrategyDistribution:   make(map[string]int64),
		EstimatorDistribution:  make(map[string]int64),
		TimeRange: struct {
			Start time.Time
			End   time.Time
		}{
			Start: startTime,
			End:   endTime,
		},
	}

	if len(metrics) == 0 {
		return summary
	}

	var totalTimeReduction time.Duration
	var totalTimeReductionPercent float64
	var totalTimeSaved time.Duration

	for _, m := range metrics {
		summary.TotalQueries++
		summary.QueriesWithCBO++

		// Count optimization results
		switch m.OptimizationResult {
		case OptimizationResultImproved:
			summary.QueriesImproved++
			totalTimeSaved += m.TimeReduction
		case OptimizationResultDegraded:
			summary.QueriesDegraded++
		case OptimizationResultNeutral:
			summary.QueriesNeutral++
		}

		// Accumulate time metrics
		if m.ExecutionTimeWithoutCBO > 0 {
			totalTimeReduction += m.TimeReduction
			totalTimeReductionPercent += m.TimeReductionPercent
		}

		// Count strategy distribution (always count, even if empty string)
		strategy := m.SelectedStrategy
		if strategy == "" {
			strategy = "unknown"
		}
		summary.StrategyDistribution[strategy]++

		// Count estimator distribution (always count, even if empty string)
		estimatorType := m.EstimatorType
		if estimatorType == "" {
			estimatorType = "unknown"
		}
		summary.EstimatorDistribution[estimatorType]++
	}

	// Calculate averages
	if summary.QueriesWithCBO > 0 {
		queriesWithBaseline := int64(0)
		for _, m := range metrics {
			if m.ExecutionTimeWithoutCBO > 0 {
				queriesWithBaseline++
			}
		}

		if queriesWithBaseline > 0 {
			summary.AvgTimeReduction = totalTimeReduction / time.Duration(queriesWithBaseline)
			summary.AvgTimeReductionPercent = totalTimeReductionPercent / float64(queriesWithBaseline)
		} else {
			// Set to 0 explicitly when no baseline data
			summary.AvgTimeReduction = 0
			summary.AvgTimeReductionPercent = 0
		}
		summary.TotalTimeSaved = totalTimeSaved
	}

	log.Ctx(ctx).Debug("CBO evaluation summary calculated",
		zap.Int64("collectionID", collectionID),
		zap.Int64("totalQueries", summary.TotalQueries),
		zap.Int64("improved", summary.QueriesImproved),
		zap.Int64("degraded", summary.QueriesDegraded),
		zap.Duration("avgTimeReduction", summary.AvgTimeReduction))

	return summary
}

// GetCBOComparison compares performance with and without CBO
func (c *CBOMetricsCollector) GetCBOComparison(
	ctx context.Context,
	collectionID int64,
	startTime, endTime time.Time,
) *CBOComparison {
	metrics := c.GetMetricsByCollection(collectionID, startTime, endTime)

	comparison := &CBOComparison{
		CollectionID: collectionID,
		TimeRange: struct {
			Start time.Time
			End   time.Time
		}{
			Start: startTime,
			End:   endTime,
		},
	}

	if len(metrics) == 0 {
		return comparison
	}

	var totalTimeWithCBO time.Duration
	var totalTimeWithoutCBO time.Duration
	var totalTimeReduction time.Duration
	var totalTimeReductionPercent float64
	var queriesWithBaseline int64

	timeReductions := make([]time.Duration, 0)

	for _, m := range metrics {
		comparison.TotalQueries++

		totalTimeWithCBO += m.ExecutionTimeWithCBO

		if m.ExecutionTimeWithoutCBO > 0 {
			queriesWithBaseline++
			totalTimeWithoutCBO += m.ExecutionTimeWithoutCBO
			totalTimeReduction += m.TimeReduction
			totalTimeReductionPercent += m.TimeReductionPercent
			timeReductions = append(timeReductions, m.TimeReduction)

			// Count optimization results
			switch m.OptimizationResult {
			case OptimizationResultImproved:
				comparison.OptimizationResults.Improved++
				comparison.TotalTimeSaved += m.TimeReduction
			case OptimizationResultDegraded:
				comparison.OptimizationResults.Degraded++
			case OptimizationResultNeutral:
				comparison.OptimizationResults.Neutral++
			}
		}
	}

	comparison.QueriesWithBaseline = queriesWithBaseline

	// Calculate averages
	if comparison.TotalQueries > 0 {
		comparison.AvgExecutionTimeWithCBO = totalTimeWithCBO / time.Duration(comparison.TotalQueries)
	}

	if queriesWithBaseline > 0 {
		comparison.AvgExecutionTimeWithoutCBO = totalTimeWithoutCBO / time.Duration(queriesWithBaseline)
		comparison.AvgTimeReduction = totalTimeReduction / time.Duration(queriesWithBaseline)
		comparison.AvgTimeReductionPercent = totalTimeReductionPercent / float64(queriesWithBaseline)

		// Calculate percentiles
		if len(timeReductions) > 0 {
			sort.Slice(timeReductions, func(i, j int) bool {
				return timeReductions[i] < timeReductions[j]
			})

			p50Index := len(timeReductions) * 50 / 100
			p95Index := len(timeReductions) * 95 / 100
			p99Index := len(timeReductions) * 99 / 100

			if p50Index < len(timeReductions) {
				comparison.PercentileMetrics.P50TimeReduction = timeReductions[p50Index]
			}
			if p95Index < len(timeReductions) {
				comparison.PercentileMetrics.P95TimeReduction = timeReductions[p95Index]
			}
			if p99Index < len(timeReductions) {
				comparison.PercentileMetrics.P99TimeReduction = timeReductions[p99Index]
			}
		}
	}

	log.Ctx(ctx).Debug("CBO comparison calculated",
		zap.Int64("collectionID", collectionID),
		zap.Int64("totalQueries", comparison.TotalQueries),
		zap.Int64("queriesWithBaseline", queriesWithBaseline),
		zap.Duration("avgTimeReduction", comparison.AvgTimeReduction),
		zap.Float64("avgTimeReductionPercent", comparison.AvgTimeReductionPercent))

	return comparison
}

// GetCBORecommendations provides recommendations based on CBO metrics
func (c *CBOMetricsCollector) GetCBORecommendations(
	ctx context.Context,
	collectionID int64,
	startTime, endTime time.Time,
) []string {
	metrics := c.GetMetricsByCollection(collectionID, startTime, endTime)
	recommendations := make([]string, 0)

	if len(metrics) == 0 {
		recommendations = append(recommendations, "No CBO metrics available for this collection. Enable CBO evaluation to get recommendations.")
		return recommendations
	}

	// Analyze degraded queries
	degradedCount := 0
	for _, m := range metrics {
		if m.OptimizationResult == OptimizationResultDegraded {
			degradedCount++
		}
	}

	if degradedCount > 0 {
		degradedPercent := float64(degradedCount) / float64(len(metrics)) * 100
		if degradedPercent > 10 {
			recommendations = append(recommendations,
				"High percentage of queries showing performance degradation. Consider reviewing CBO selectivity threshold or estimator configuration.")
		}
	}

	// Check estimator usage
	statisticsEstimatorCount := 0
	for _, m := range metrics {
		if m.EstimatorType == "statistics" {
			statisticsEstimatorCount++
		}
	}

	if statisticsEstimatorCount == 0 {
		recommendations = append(recommendations,
			"Statistics-based estimator is not being used. Consider enabling it for more accurate selectivity estimates.")
	}

	// Check strategy distribution
	strategyCounts := make(map[string]int)
	for _, m := range metrics {
		strategyCounts[m.SelectedStrategy]++
	}

	if len(strategyCounts) > 0 {
		// Check if one strategy dominates
		maxCount := 0
		for _, count := range strategyCounts {
			if count > maxCount {
				maxCount = count
			}
		}

		if float64(maxCount)/float64(len(metrics)) > 0.9 {
			recommendations = append(recommendations,
				"One filter strategy is being used for almost all queries. Consider reviewing selectivity threshold.")
		}
	}

	// Check for queries with very high selectivity estimates
	highSelectivityCount := 0
	for _, m := range metrics {
		if m.SelectivityEstimate > 0.5 {
			highSelectivityCount++
		}
	}

	if highSelectivityCount > len(metrics)/2 {
		recommendations = append(recommendations,
			"Many queries have high selectivity estimates. Consider reviewing field statistics or estimator configuration.")
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "CBO is performing well. No specific recommendations at this time.")
	}

	return recommendations
}

// CBOOverview represents a comprehensive overview combining summary and comparison
type CBOOverview struct {
	CollectionID     int64
	CollectionName   string
	TimeRange        struct {
		Start time.Time
		End   time.Time
	}

	// Totals
	TotalQueries      int64
	QueriesWithCBO    int64
	QueriesWithBaseline int64

	// Optimization Results
	QueriesImproved int64
	QueriesDegraded  int64
	QueriesNeutral   int64

	// Time Metrics
	AvgTimeReduction        time.Duration
	AvgTimeReductionPercent float64
	TotalTimeSaved          time.Duration

	// Percentiles (from comparison)
	PercentileMetrics struct {
		P50TimeReduction time.Duration
		P95TimeReduction time.Duration
		P99TimeReduction time.Duration
	}

	// Distributions
	StrategyDistribution  map[string]int64
	EstimatorDistribution  map[string]int64

	// Baseline Coverage
	BaselineCoveragePercent float64
}

// GetCBOOverview returns a comprehensive overview combining summary and comparison
func (c *CBOMetricsCollector) GetCBOOverview(
	ctx context.Context,
	collectionID int64,
	startTime, endTime time.Time,
) *CBOOverview {
	summary := c.GetCBOEvaluationSummary(ctx, collectionID, startTime, endTime)
	comparison := c.GetCBOComparison(ctx, collectionID, startTime, endTime)

	overview := &CBOOverview{
		CollectionID:     collectionID,
		TimeRange: struct {
			Start time.Time
			End   time.Time
		}{
			Start: startTime,
			End:   endTime,
		},
		TotalQueries:            summary.TotalQueries,
		QueriesWithCBO:          summary.QueriesWithCBO,
		QueriesWithBaseline:     comparison.QueriesWithBaseline,
		QueriesImproved:         summary.QueriesImproved,
		QueriesDegraded:         summary.QueriesDegraded,
		QueriesNeutral:          summary.QueriesNeutral,
		AvgTimeReduction:        summary.AvgTimeReduction,
		AvgTimeReductionPercent: summary.AvgTimeReductionPercent,
		TotalTimeSaved:          summary.TotalTimeSaved,
		PercentileMetrics:       comparison.PercentileMetrics,
		StrategyDistribution:    summary.StrategyDistribution,
		EstimatorDistribution:   summary.EstimatorDistribution,
	}

	// Calculate baseline coverage
	if overview.QueriesWithCBO > 0 {
		overview.BaselineCoveragePercent = float64(overview.QueriesWithBaseline) / float64(overview.QueriesWithCBO) * 100
	}

	// Get collection name from first metric if available
	metrics := c.GetMetricsByCollection(collectionID, startTime, endTime)
	if len(metrics) > 0 && metrics[0] != nil {
		overview.CollectionName = metrics[0].CollectionName
	}

	return overview
}

// GetCBOTop returns top-N queries sorted by improvement or degradation
func (c *CBOMetricsCollector) GetCBOTop(
	ctx context.Context,
	collectionID int64,
	order string, // "degraded" or "improved"
	limit int,
	startTime, endTime time.Time,
) []*CBOMetrics {
	metrics := c.GetMetricsByCollection(collectionID, startTime, endTime)

	// Filter to only queries with baseline data
	withBaseline := make([]*CBOMetrics, 0)
	for _, m := range metrics {
		if m.ExecutionTimeWithoutCBO > 0 {
			withBaseline = append(withBaseline, m)
		}
	}

	if len(withBaseline) == 0 {
		return []*CBOMetrics{}
	}

	// Sort by improvement percent
	if order == "improved" {
		sort.Slice(withBaseline, func(i, j int) bool {
			return withBaseline[i].ImprovementPercent > withBaseline[j].ImprovementPercent
		})
	} else {
		// Default to degraded (worst first)
		sort.Slice(withBaseline, func(i, j int) bool {
			return withBaseline[i].ImprovementPercent < withBaseline[j].ImprovementPercent
		})
	}

	// Apply limit
	if limit > 0 && limit < len(withBaseline) {
		withBaseline = withBaseline[:limit]
	}

	return withBaseline
}

