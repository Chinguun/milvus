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
	"time"
)

// OptimizationResult represents the result of CBO optimization
type OptimizationResult string

const (
	OptimizationResultImproved OptimizationResult = "improved"
	OptimizationResultDegraded  OptimizationResult = "degraded"
	OptimizationResultNeutral   OptimizationResult = "neutral"
)

// CBOMetrics represents comprehensive metrics for a single query with CBO
type CBOMetrics struct {
	QueryID          string
	CollectionID     int64
	CollectionName   string
	FilterExpression string
	Timestamp        time.Time

	// CBO Decision
	SelectivityEstimate float64
	SelectedStrategy    string // "standard_filter" or "iterative_filter"
	EstimatorType       string // "mock" or "statistics"

	// Decision Metadata
	DecisionApplied    bool    // Whether CBO decision was applied (vs skipped)
	DecisionSkipReason string  // Reason for skipping: "hints", "no_filter_expr", "disabled", "unknown"
	SelectivityThreshold float64 // Threshold used for decision (0.0-1.0)
	HintsBefore        string  // Hints before CBO (optional)
	HintsAfter         string  // Hints after CBO (optional)

	// Baseline Metadata
	BaselineSampled   bool   // Whether this query was sampled for baseline comparison
	BaselineAttempted bool   // Whether baseline execution was attempted
	BaselineStatus    string // Baseline execution status: "success", "timeout", "error"
	BaselineErrorCode string // Error code if baseline failed (optional)
	BaselineErrorMsg  string // Truncated error message if baseline failed (optional)

	// Performance Metrics
	ExecutionTimeWithCBO    time.Duration
	ExecutionTimeWithoutCBO time.Duration
	TimeReduction           time.Duration
	TimeReductionPercent    float64

	// Query Details
	NumQueries int
	TopK       int64
	NumResults int64

	// Optimization Result
	OptimizationResult OptimizationResult
	ImprovementPercent float64
}

// CalculateOptimizationResult calculates and sets the optimization result based on execution times
func (m *CBOMetrics) CalculateOptimizationResult() {
	if m.ExecutionTimeWithoutCBO == 0 {
		m.OptimizationResult = OptimizationResultNeutral
		m.ImprovementPercent = 0
		return
	}

	if m.ExecutionTimeWithCBO < m.ExecutionTimeWithoutCBO {
		m.OptimizationResult = OptimizationResultImproved
		m.TimeReduction = m.ExecutionTimeWithoutCBO - m.ExecutionTimeWithCBO
		m.TimeReductionPercent = (float64(m.TimeReduction) / float64(m.ExecutionTimeWithoutCBO)) * 100
		m.ImprovementPercent = m.TimeReductionPercent
	} else if m.ExecutionTimeWithCBO > m.ExecutionTimeWithoutCBO {
		m.OptimizationResult = OptimizationResultDegraded
		m.TimeReduction = m.ExecutionTimeWithCBO - m.ExecutionTimeWithoutCBO
		m.TimeReductionPercent = (float64(m.TimeReduction) / float64(m.ExecutionTimeWithoutCBO)) * 100
		m.ImprovementPercent = -m.TimeReductionPercent
	} else {
		m.OptimizationResult = OptimizationResultNeutral
		m.TimeReduction = 0
		m.TimeReductionPercent = 0
		m.ImprovementPercent = 0
	}
}

// CBOEvaluationSummary represents aggregated statistics for CBO evaluation
type CBOEvaluationSummary struct {
	TotalQueries      int64
	QueriesWithCBO    int64
	QueriesImproved   int64
	QueriesDegraded   int64
	QueriesNeutral    int64

	AvgTimeReduction        time.Duration
	AvgTimeReductionPercent float64
	TotalTimeSaved          time.Duration

	StrategyDistribution  map[string]int64
	EstimatorDistribution map[string]int64

	TimeRange struct {
		Start time.Time
		End   time.Time
	}
}

// CBOComparison represents a comparison between queries with and without CBO
type CBOComparison struct {
	CollectionID     int64
	CollectionName   string
	TimeRange        struct {
		Start time.Time
		End   time.Time
	}

	TotalQueries            int64
	QueriesWithBaseline     int64

	AvgExecutionTimeWithCBO    time.Duration
	AvgExecutionTimeWithoutCBO  time.Duration
	AvgTimeReduction           time.Duration
	AvgTimeReductionPercent    float64
	TotalTimeSaved             time.Duration

	PercentileMetrics struct {
		P50TimeReduction time.Duration
		P95TimeReduction time.Duration
		P99TimeReduction time.Duration
	}

	OptimizationResults struct {
		Improved int64
		Degraded int64
		Neutral  int64
	}
}

