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
	"math"

	"go.uber.org/zap"

	"github.com/milvus-io/milvus-proto/go-api/v2/schemapb"
	"github.com/milvus-io/milvus/pkg/v2/log"
	"github.com/milvus-io/milvus/pkg/v2/proto/planpb"
)

const (
	// DefaultSelectivityThreshold defines the default threshold for choosing standard filtering vs iterative filtering
	// If selectivity < threshold: use standard filtering (build BitSet)
	// If selectivity >= threshold: use iterative filtering (iterative filter)
	// Note: This default is overridden by proxy.cbo.selectivityThreshold configuration parameter when set.
	DefaultSelectivityThreshold = 0.05 // 5%

	// IterativeFilterHint is the hint value to enable iterative filtering
	IterativeFilterHint = "iterative_filter"
	// DisableIterativeFilterHint is the hint value to disable iterative filtering (force standard filtering)
	DisableIterativeFilterHint = "disable"
)

// SelectivityEstimator is the interface for estimating filter expression selectivity
type SelectivityEstimator interface {
	// EstimateSelectivity estimates the selectivity of a filter expression
	// Returns a value between 0.0 and 1.0, where:
	// - 0.0 means no rows match (highly selective)
	// - 1.0 means all rows match (not selective)
	EstimateSelectivity(expr *planpb.Expr, schema *schemapb.CollectionSchema) float64
}

// MockSelectivityEstimator provides a simple rule-based selectivity estimation
// This is a placeholder implementation for the assignment. In production, this would
// use histograms, statistics, or other advanced techniques.
type MockSelectivityEstimator struct {
	// FieldSelectivityMap maps field names to their estimated selectivity
	// Selectivity is a value between 0.0 and 1.0, where:
	// - 0.0 means no rows match (highly selective)
	// - 1.0 means all rows match (not selective)
	FieldSelectivityMap map[string]float64
}

// NewMockSelectivityEstimator creates a new mock estimator with default rules
func NewMockSelectivityEstimator() *MockSelectivityEstimator {
	return &MockSelectivityEstimator{
		FieldSelectivityMap: map[string]float64{
			// Example rules: price field typically has low selectivity (highly selective)
			"price": 0.001, // 0.1% selectivity - very selective
			// category field typically has higher selectivity
			"category": 0.1, // 10% selectivity - moderately selective
			// Default selectivity for unknown fields
			"default": 0.3, // 30% selectivity - assume moderate selectivity
		},
	}
}

// EstimateSelectivity estimates the selectivity of a filter expression
// Returns a value between 0.0 and 1.0
func (e *MockSelectivityEstimator) EstimateSelectivity(
	expr *planpb.Expr,
	schema *schemapb.CollectionSchema,
) float64 {
	if expr == nil {
		// No filter means all rows match
		return 1.0
	}

	// Extract field IDs from the expression
	fieldIDs := extractFieldIDsFromExpr(expr)

	if len(fieldIDs) == 0 {
		// No fields found, assume moderate selectivity
		return e.FieldSelectivityMap["default"]
	}

	// For multiple fields, use the minimum selectivity (most selective field wins)
	// This is a simple heuristic - in production, you'd consider AND/OR logic
	minSelectivity := 1.0
	for _, fieldID := range fieldIDs {
		fieldName := getFieldNameByID(schema, fieldID)
		if fieldName == "" {
			continue
		}

		selectivity, exists := e.FieldSelectivityMap[fieldName]
		if !exists {
			selectivity = e.FieldSelectivityMap["default"]
		}

		if selectivity < minSelectivity {
			minSelectivity = selectivity
		}
	}

	return minSelectivity
}

// extractFieldIDsFromExpr recursively extracts all field IDs from an expression
func extractFieldIDsFromExpr(expr *planpb.Expr) []int64 {
	if expr == nil {
		return nil
	}

	var fieldIDs []int64

	switch e := expr.Expr.(type) {
	case *planpb.Expr_TermExpr:
		if e.TermExpr != nil && e.TermExpr.ColumnInfo != nil {
			fieldIDs = append(fieldIDs, e.TermExpr.ColumnInfo.FieldId)
		}

	case *planpb.Expr_UnaryRangeExpr:
		if e.UnaryRangeExpr != nil && e.UnaryRangeExpr.ColumnInfo != nil {
			fieldIDs = append(fieldIDs, e.UnaryRangeExpr.ColumnInfo.FieldId)
		}

	case *planpb.Expr_BinaryRangeExpr:
		if e.BinaryRangeExpr != nil && e.BinaryRangeExpr.ColumnInfo != nil {
			fieldIDs = append(fieldIDs, e.BinaryRangeExpr.ColumnInfo.FieldId)
		}

	case *planpb.Expr_BinaryArithOpEvalRangeExpr:
		if e.BinaryArithOpEvalRangeExpr != nil && e.BinaryArithOpEvalRangeExpr.ColumnInfo != nil {
			fieldIDs = append(fieldIDs, e.BinaryArithOpEvalRangeExpr.ColumnInfo.FieldId)
		}

	case *planpb.Expr_CompareExpr:
		if e.CompareExpr != nil {
			if e.CompareExpr.LeftColumnInfo != nil {
				fieldIDs = append(fieldIDs, e.CompareExpr.LeftColumnInfo.FieldId)
			}
			if e.CompareExpr.RightColumnInfo != nil {
				fieldIDs = append(fieldIDs, e.CompareExpr.RightColumnInfo.FieldId)
			}
		}

	case *planpb.Expr_UnaryExpr:
		if e.UnaryExpr != nil && e.UnaryExpr.Child != nil {
			fieldIDs = append(fieldIDs, extractFieldIDsFromExpr(e.UnaryExpr.Child)...)
		}

	case *planpb.Expr_BinaryExpr:
		if e.BinaryExpr != nil {
			if e.BinaryExpr.Left != nil {
				fieldIDs = append(fieldIDs, extractFieldIDsFromExpr(e.BinaryExpr.Left)...)
			}
			if e.BinaryExpr.Right != nil {
				fieldIDs = append(fieldIDs, extractFieldIDsFromExpr(e.BinaryExpr.Right)...)
			}
		}

	case *planpb.Expr_JsonContainsExpr:
		if e.JsonContainsExpr != nil && e.JsonContainsExpr.ColumnInfo != nil {
			fieldIDs = append(fieldIDs, e.JsonContainsExpr.ColumnInfo.FieldId)
		}

	case *planpb.Expr_NullExpr:
		if e.NullExpr != nil && e.NullExpr.ColumnInfo != nil {
			fieldIDs = append(fieldIDs, e.NullExpr.ColumnInfo.FieldId)
		}

	case *planpb.Expr_GisfunctionFilterExpr:
		if e.GisfunctionFilterExpr != nil && e.GisfunctionFilterExpr.ColumnInfo != nil {
			fieldIDs = append(fieldIDs, e.GisfunctionFilterExpr.ColumnInfo.FieldId)
		}

	case *planpb.Expr_TimestamptzArithCompareExpr:
		if e.TimestamptzArithCompareExpr != nil && e.TimestamptzArithCompareExpr.TimestamptzColumn != nil {
			fieldIDs = append(fieldIDs, e.TimestamptzArithCompareExpr.TimestamptzColumn.FieldId)
		}

	case *planpb.Expr_CallExpr:
		if e.CallExpr != nil {
			for _, param := range e.CallExpr.FunctionParameters {
				fieldIDs = append(fieldIDs, extractFieldIDsFromExpr(param)...)
			}
		}

	case *planpb.Expr_RandomSampleExpr:
		if e.RandomSampleExpr != nil && e.RandomSampleExpr.Predicate != nil {
			fieldIDs = append(fieldIDs, extractFieldIDsFromExpr(e.RandomSampleExpr.Predicate)...)
		}
	}

	// Remove duplicates
	seen := make(map[int64]bool)
	uniqueFieldIDs := []int64{}
	for _, id := range fieldIDs {
		if !seen[id] {
			seen[id] = true
			uniqueFieldIDs = append(uniqueFieldIDs, id)
		}
	}

	return uniqueFieldIDs
}

// getFieldNameByID retrieves the field name from schema by field ID
func getFieldNameByID(schema *schemapb.CollectionSchema, fieldID int64) string {
	if schema == nil {
		return ""
	}

	for _, field := range schema.Fields {
		if field.FieldID == fieldID {
			return field.Name
		}
	}

	return ""
}

// StatisticsBasedSelectivityEstimator uses field statistics to estimate selectivity
type StatisticsBasedSelectivityEstimator struct {
	accessor      StatisticsAccessor
	collectionID  int64
	fallbackEstimator SelectivityEstimator // Fallback when statistics unavailable
	defaultSelectivity float64              // Default selectivity when no stats available
}

// NewStatisticsBasedSelectivityEstimator creates a new statistics-based estimator
func NewStatisticsBasedSelectivityEstimator(
	accessor StatisticsAccessor,
	collectionID int64,
	fallbackEstimator SelectivityEstimator,
) *StatisticsBasedSelectivityEstimator {
	if fallbackEstimator == nil {
		fallbackEstimator = NewMockSelectivityEstimator()
	}
	return &StatisticsBasedSelectivityEstimator{
		accessor:          accessor,
		collectionID:       collectionID,
		fallbackEstimator:  fallbackEstimator,
		defaultSelectivity: 0.3, // 30% default selectivity
	}
}

// EstimateSelectivity estimates selectivity using field statistics
func (e *StatisticsBasedSelectivityEstimator) EstimateSelectivity(
	expr *planpb.Expr,
	schema *schemapb.CollectionSchema,
) float64 {
	if expr == nil {
		return 1.0
	}

	ctx := context.Background()
	return e.estimateSelectivityRecursive(ctx, expr, schema)
}

// estimateSelectivityRecursive recursively estimates selectivity for complex expressions
func (e *StatisticsBasedSelectivityEstimator) estimateSelectivityRecursive(
	ctx context.Context,
	expr *planpb.Expr,
	schema *schemapb.CollectionSchema,
) float64 {
	if expr == nil {
		return 1.0
	}

	switch exprType := expr.Expr.(type) {
	case *planpb.Expr_UnaryRangeExpr:
		return e.estimateUnaryRange(ctx, exprType.UnaryRangeExpr, schema)

	case *planpb.Expr_BinaryRangeExpr:
		return e.estimateBinaryRange(ctx, exprType.BinaryRangeExpr, schema)

	case *planpb.Expr_TermExpr:
		return e.estimateTerm(ctx, exprType.TermExpr, schema)

	case *planpb.Expr_CompareExpr:
		return e.estimateCompare(ctx, exprType.CompareExpr, schema)

	case *planpb.Expr_BinaryExpr:
		return e.estimateBinary(ctx, exprType.BinaryExpr, schema)

	case *planpb.Expr_UnaryExpr:
		// NOT operator: selectivity = 1 - child_selectivity
		childSelectivity := e.estimateSelectivityRecursive(ctx, exprType.UnaryExpr.Child, schema)
		return 1.0 - childSelectivity

	default:
		// For other expression types, fall back to fallback estimator
		log.Ctx(ctx).Debug("CBO: Using fallback estimator for unsupported expression type",
			zap.String("type", e.getExprTypeName(expr)))
		return e.fallbackEstimator.EstimateSelectivity(expr, schema)
	}
}

// estimateUnaryRange estimates selectivity for unary range expressions (price < 10, price > 20)
func (e *StatisticsBasedSelectivityEstimator) estimateUnaryRange(
	ctx context.Context,
	expr *planpb.UnaryRangeExpr,
	schema *schemapb.CollectionSchema,
) float64 {
	if expr == nil || expr.ColumnInfo == nil {
		return e.defaultSelectivity
	}

	fieldID := expr.ColumnInfo.FieldId
	stats, err := e.accessor.GetFieldStatistics(ctx, e.collectionID, fieldID)
	if err != nil || stats == nil || stats.Min == nil || stats.Max == nil {
		log.Ctx(ctx).Debug("CBO: Statistics not available for unary range, using fallback",
			zap.Int64("fieldID", fieldID))
		return e.fallbackEstimator.EstimateSelectivity(&planpb.Expr{
			Expr: &planpb.Expr_UnaryRangeExpr{UnaryRangeExpr: expr},
		}, schema)
	}

	// Extract value from GenericValue
	threshold, ok := e.extractNumericValue(expr.Value)
	if !ok {
		return e.defaultSelectivity
	}

	minVal, maxVal, ok := e.extractMinMax(stats, expr.ColumnInfo.DataType)
	if !ok {
		return e.defaultSelectivity
	}

	// Calculate selectivity based on operator type
	switch expr.Op {
	case planpb.OpType_LessThan, planpb.OpType_LessEqual:
		if threshold <= minVal {
			return 0.0
		}
		if threshold >= maxVal {
			return 1.0
		}
		return (threshold - minVal) / (maxVal - minVal)

	case planpb.OpType_GreaterThan, planpb.OpType_GreaterEqual:
		if threshold >= maxVal {
			return 0.0
		}
		if threshold <= minVal {
			return 1.0
		}
		return (maxVal - threshold) / (maxVal - minVal)

	default:
		return e.defaultSelectivity
	}
}

// estimateBinaryRange estimates selectivity for binary range expressions (price BETWEEN 10 AND 20)
func (e *StatisticsBasedSelectivityEstimator) estimateBinaryRange(
	ctx context.Context,
	expr *planpb.BinaryRangeExpr,
	schema *schemapb.CollectionSchema,
) float64 {
	if expr == nil || expr.ColumnInfo == nil {
		return e.defaultSelectivity
	}

	fieldID := expr.ColumnInfo.FieldId
	stats, err := e.accessor.GetFieldStatistics(ctx, e.collectionID, fieldID)
	if err != nil || stats == nil || stats.Min == nil || stats.Max == nil {
		log.Ctx(ctx).Debug("CBO: Statistics not available for binary range, using fallback",
			zap.Int64("fieldID", fieldID))
		return e.fallbackEstimator.EstimateSelectivity(&planpb.Expr{
			Expr: &planpb.Expr_BinaryRangeExpr{BinaryRangeExpr: expr},
		}, schema)
	}

	lower, ok1 := e.extractNumericValue(expr.LowerValue)
	upper, ok2 := e.extractNumericValue(expr.UpperValue)
	if !ok1 || !ok2 {
		return e.defaultSelectivity
	}

	minVal, maxVal, ok := e.extractMinMax(stats, expr.ColumnInfo.DataType)
	if !ok {
		return e.defaultSelectivity
	}

	// Adjust bounds based on inclusivity
	if !expr.LowerInclusive {
		lower = lower + 1e-10 // Small epsilon for exclusive bounds
	}
	if !expr.UpperInclusive {
		upper = upper - 1e-10
	}

	// Clamp to valid range
	if lower < minVal {
		lower = minVal
	}
	if upper > maxVal {
		upper = maxVal
	}
	if lower > upper {
		return 0.0
	}

	if maxVal == minVal {
		return 1.0
	}

	return (upper - lower) / (maxVal - minVal)
}

// estimateTerm estimates selectivity for term expressions (category IN [1, 2, 3])
func (e *StatisticsBasedSelectivityEstimator) estimateTerm(
	ctx context.Context,
	expr *planpb.TermExpr,
	schema *schemapb.CollectionSchema,
) float64 {
	if expr == nil || expr.ColumnInfo == nil || len(expr.Values) == 0 {
		return e.defaultSelectivity
	}

	fieldID := expr.ColumnInfo.FieldId
	stats, err := e.accessor.GetFieldStatistics(ctx, e.collectionID, fieldID)
	if err != nil || stats == nil {
		log.Ctx(ctx).Debug("CBO: Statistics not available for term, using fallback",
			zap.Int64("fieldID", fieldID))
		return e.fallbackEstimator.EstimateSelectivity(&planpb.Expr{
			Expr: &planpb.Expr_TermExpr{TermExpr: expr},
		}, schema)
	}

	// Use cardinality if available, otherwise estimate
	cardinality := stats.Cardinality
	if cardinality <= 0 {
		// Estimate cardinality based on row count (assume uniform distribution)
		cardinality = stats.RowCount / 10 // Rough estimate
		if cardinality <= 0 {
			cardinality = 100 // Default fallback
		}
	}

	numTerms := float64(len(expr.Values))
	selectivity := numTerms / float64(cardinality)
	return math.Min(1.0, selectivity)
}

// estimateCompare estimates selectivity for compare expressions (price == 10)
func (e *StatisticsBasedSelectivityEstimator) estimateCompare(
	ctx context.Context,
	expr *planpb.CompareExpr,
	schema *schemapb.CollectionSchema,
) float64 {
	if expr == nil {
		return e.defaultSelectivity
	}

	// For equality comparisons, estimate based on cardinality
	if expr.Op == planpb.OpType_Equal && expr.LeftColumnInfo != nil {
		fieldID := expr.LeftColumnInfo.FieldId
		stats, err := e.accessor.GetFieldStatistics(ctx, e.collectionID, fieldID)
		if err != nil || stats == nil {
			log.Ctx(ctx).Debug("CBO: Statistics not available for compare, using fallback",
				zap.Int64("fieldID", fieldID))
			return e.fallbackEstimator.EstimateSelectivity(&planpb.Expr{
				Expr: &planpb.Expr_CompareExpr{CompareExpr: expr},
			}, schema)
		}

		cardinality := stats.Cardinality
		if cardinality <= 0 {
			cardinality = stats.RowCount / 10
			if cardinality <= 0 {
				cardinality = 100
			}
		}

		return 1.0 / float64(cardinality)
	}

	// For other comparison types, use fallback
	return e.fallbackEstimator.EstimateSelectivity(&planpb.Expr{
		Expr: &planpb.Expr_CompareExpr{CompareExpr: expr},
	}, schema)
}

// estimateBinary estimates selectivity for binary expressions (AND, OR)
func (e *StatisticsBasedSelectivityEstimator) estimateBinary(
	ctx context.Context,
	expr *planpb.BinaryExpr,
	schema *schemapb.CollectionSchema,
) float64 {
	if expr == nil {
		return e.defaultSelectivity
	}

	leftSelectivity := e.estimateSelectivityRecursive(ctx, expr.Left, schema)
	rightSelectivity := e.estimateSelectivityRecursive(ctx, expr.Right, schema)

	switch expr.Op {
	case planpb.BinaryExpr_LogicalAnd:
		// AND: P(A AND B) = P(A) * P(B) (assuming independence)
		return leftSelectivity * rightSelectivity

	case planpb.BinaryExpr_LogicalOr:
		// OR: P(A OR B) = 1 - (1 - P(A)) * (1 - P(B))
		return 1.0 - (1.0-leftSelectivity)*(1.0-rightSelectivity)

	default:
		return e.defaultSelectivity
	}
}

// Helper functions

func (e *StatisticsBasedSelectivityEstimator) extractNumericValue(value *planpb.GenericValue) (float64, bool) {
	if value == nil {
		return 0, false
	}

	switch v := value.Val.(type) {
	case *planpb.GenericValue_Int64Val:
		return float64(v.Int64Val), true
	case *planpb.GenericValue_FloatVal:
		return v.FloatVal, true
	default:
		return 0, false
	}
}

func (e *StatisticsBasedSelectivityEstimator) extractMinMax(
	stats *FieldStatistics,
	dataType schemapb.DataType,
) (float64, float64, bool) {
	if stats.Min == nil || stats.Max == nil {
		return 0, 0, false
	}

	var minVal, maxVal float64

	switch dataType {
	case schemapb.DataType_Int8:
		if v, ok := stats.Min.(int8); ok {
			minVal = float64(v)
			if v, ok := stats.Max.(int8); ok {
				maxVal = float64(v)
				return minVal, maxVal, true
			}
		}
	case schemapb.DataType_Int16:
		if v, ok := stats.Min.(int16); ok {
			minVal = float64(v)
			if v, ok := stats.Max.(int16); ok {
				maxVal = float64(v)
				return minVal, maxVal, true
			}
		}
	case schemapb.DataType_Int32:
		if v, ok := stats.Min.(int32); ok {
			minVal = float64(v)
			if v, ok := stats.Max.(int32); ok {
				maxVal = float64(v)
				return minVal, maxVal, true
			}
		}
	case schemapb.DataType_Int64:
		if v, ok := stats.Min.(int64); ok {
			minVal = float64(v)
			if v, ok := stats.Max.(int64); ok {
				maxVal = float64(v)
				return minVal, maxVal, true
			}
		}
	case schemapb.DataType_Float:
		if v, ok := stats.Min.(float32); ok {
			minVal = float64(v)
			if v, ok := stats.Max.(float32); ok {
				maxVal = float64(v)
				return minVal, maxVal, true
			}
		}
	case schemapb.DataType_Double:
		if v, ok := stats.Min.(float64); ok {
			minVal = v
			if v, ok := stats.Max.(float64); ok {
				maxVal = v
				return minVal, maxVal, true
			}
		}
	}

	return 0, 0, false
}

func (e *StatisticsBasedSelectivityEstimator) getExprTypeName(expr *planpb.Expr) string {
	if expr == nil {
		return "nil"
	}
	switch expr.Expr.(type) {
	case *planpb.Expr_UnaryRangeExpr:
		return "UnaryRangeExpr"
	case *planpb.Expr_BinaryRangeExpr:
		return "BinaryRangeExpr"
	case *planpb.Expr_TermExpr:
		return "TermExpr"
	case *planpb.Expr_CompareExpr:
		return "CompareExpr"
	case *planpb.Expr_BinaryExpr:
		return "BinaryExpr"
	case *planpb.Expr_UnaryExpr:
		return "UnaryExpr"
	default:
		return "Unknown"
	}
}

// DecideFilterStrategy decides whether to use standard filtering or iterative filtering
// based on estimated selectivity
// Returns:
//   - true if iterative filtering should be used
//   - false if standard filtering (BitSet) should be used
func DecideFilterStrategy(
	selectivity float64,
	threshold float64,
) bool {
	// If selectivity >= threshold, use iterative filtering
	// If selectivity < threshold, use standard filtering (BitSet)
	return selectivity >= threshold
}
