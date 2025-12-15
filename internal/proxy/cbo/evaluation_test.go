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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCalculateOptimizationResult_Improved(t *testing.T) {
	metrics := &CBOMetrics{
		ExecutionTimeWithCBO:    50 * time.Millisecond,
		ExecutionTimeWithoutCBO: 100 * time.Millisecond,
	}

	metrics.CalculateOptimizationResult()

	assert.Equal(t, OptimizationResultImproved, metrics.OptimizationResult)
	assert.Equal(t, 50*time.Millisecond, metrics.TimeReduction)
	assert.Equal(t, 50.0, metrics.TimeReductionPercent)
	assert.Equal(t, 50.0, metrics.ImprovementPercent)
}

func TestCalculateOptimizationResult_Degraded(t *testing.T) {
	metrics := &CBOMetrics{
		ExecutionTimeWithCBO:    150 * time.Millisecond,
		ExecutionTimeWithoutCBO: 100 * time.Millisecond,
	}

	metrics.CalculateOptimizationResult()

	assert.Equal(t, OptimizationResultDegraded, metrics.OptimizationResult)
	assert.Equal(t, 50*time.Millisecond, metrics.TimeReduction)
	assert.Equal(t, 50.0, metrics.TimeReductionPercent)
	assert.Equal(t, -50.0, metrics.ImprovementPercent)
}

func TestCalculateOptimizationResult_Neutral(t *testing.T) {
	metrics := &CBOMetrics{
		ExecutionTimeWithCBO:    100 * time.Millisecond,
		ExecutionTimeWithoutCBO: 100 * time.Millisecond,
	}

	metrics.CalculateOptimizationResult()

	assert.Equal(t, OptimizationResultNeutral, metrics.OptimizationResult)
	assert.Equal(t, time.Duration(0), metrics.TimeReduction)
	assert.Equal(t, 0.0, metrics.TimeReductionPercent)
	assert.Equal(t, 0.0, metrics.ImprovementPercent)
}

func TestCalculateOptimizationResult_NoBaseline(t *testing.T) {
	metrics := &CBOMetrics{
		ExecutionTimeWithCBO:    50 * time.Millisecond,
		ExecutionTimeWithoutCBO: 0,
	}

	metrics.CalculateOptimizationResult()

	assert.Equal(t, OptimizationResultNeutral, metrics.OptimizationResult)
	assert.Equal(t, 0.0, metrics.ImprovementPercent)
	assert.Equal(t, time.Duration(0), metrics.TimeReduction)
	assert.Equal(t, 0.0, metrics.TimeReductionPercent)
}

func TestCalculateOptimizationResult_EdgeCases(t *testing.T) {
	t.Run("very small durations", func(t *testing.T) {
		metrics := &CBOMetrics{
			ExecutionTimeWithCBO:    1 * time.Nanosecond,
			ExecutionTimeWithoutCBO: 2 * time.Nanosecond,
		}

		metrics.CalculateOptimizationResult()

		assert.Equal(t, OptimizationResultImproved, metrics.OptimizationResult)
		assert.Equal(t, 1*time.Nanosecond, metrics.TimeReduction)
		assert.InDelta(t, 50.0, metrics.TimeReductionPercent, 0.01)
	})

	t.Run("very large durations", func(t *testing.T) {
		metrics := &CBOMetrics{
			ExecutionTimeWithCBO:    1 * time.Hour,
			ExecutionTimeWithoutCBO: 2 * time.Hour,
		}

		metrics.CalculateOptimizationResult()

		assert.Equal(t, OptimizationResultImproved, metrics.OptimizationResult)
		assert.Equal(t, 1*time.Hour, metrics.TimeReduction)
		assert.Equal(t, 50.0, metrics.TimeReductionPercent)
	})

	t.Run("zero with CBO", func(t *testing.T) {
		metrics := &CBOMetrics{
			ExecutionTimeWithCBO:    0,
			ExecutionTimeWithoutCBO: 100 * time.Millisecond,
		}

		metrics.CalculateOptimizationResult()

		assert.Equal(t, OptimizationResultImproved, metrics.OptimizationResult)
		assert.Equal(t, 100*time.Millisecond, metrics.TimeReduction)
		assert.Equal(t, 100.0, metrics.TimeReductionPercent)
	})

	t.Run("both zero", func(t *testing.T) {
		metrics := &CBOMetrics{
			ExecutionTimeWithCBO:    0,
			ExecutionTimeWithoutCBO: 0,
		}

		metrics.CalculateOptimizationResult()

		assert.Equal(t, OptimizationResultNeutral, metrics.OptimizationResult)
		assert.Equal(t, time.Duration(0), metrics.TimeReduction)
		assert.Equal(t, 0.0, metrics.TimeReductionPercent)
	})

	t.Run("small improvement percentage", func(t *testing.T) {
		metrics := &CBOMetrics{
			ExecutionTimeWithCBO:    99 * time.Millisecond,
			ExecutionTimeWithoutCBO: 100 * time.Millisecond,
		}

		metrics.CalculateOptimizationResult()

		assert.Equal(t, OptimizationResultImproved, metrics.OptimizationResult)
		assert.Equal(t, 1*time.Millisecond, metrics.TimeReduction)
		assert.Equal(t, 1.0, metrics.TimeReductionPercent)
	})

	t.Run("large improvement percentage", func(t *testing.T) {
		metrics := &CBOMetrics{
			ExecutionTimeWithCBO:    10 * time.Millisecond,
			ExecutionTimeWithoutCBO: 100 * time.Millisecond,
		}

		metrics.CalculateOptimizationResult()

		assert.Equal(t, OptimizationResultImproved, metrics.OptimizationResult)
		assert.Equal(t, 90*time.Millisecond, metrics.TimeReduction)
		assert.Equal(t, 90.0, metrics.TimeReductionPercent)
	})
}

