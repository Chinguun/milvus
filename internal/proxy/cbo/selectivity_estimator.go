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
	"github.com/milvus-io/milvus-proto/go-api/v2/schemapb"
	"github.com/milvus-io/milvus/pkg/v2/proto/planpb"
)

const (
	// DefaultSelectivityThreshold defines the threshold for choosing pre-filtering vs post-filtering
	// If selectivity < threshold: use pre-filtering (build BitSet)
	// If selectivity >= threshold: use post-filtering (iterative filter)
	DefaultSelectivityThreshold = 0.05 // 5%

	// IterativeFilterHint is the hint value to enable iterative (post) filtering
	IterativeFilterHint = "iterative_filter"
	// DisableIterativeFilterHint is the hint value to disable iterative filtering (force pre-filtering)
	DisableIterativeFilterHint = "disable"
)

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

// DecideFilterStrategy decides whether to use pre-filtering or post-filtering
// based on estimated selectivity
// Returns:
//   - true if post-filtering (iterative filter) should be used
//   - false if pre-filtering (BitSet) should be used
func DecideFilterStrategy(
	selectivity float64,
	threshold float64,
) bool {
	// If selectivity >= threshold, use post-filtering (iterative)
	// If selectivity < threshold, use pre-filtering (BitSet)
	return selectivity >= threshold
}

