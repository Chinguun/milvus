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
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/milvus-io/milvus-proto/go-api/v2/schemapb"
	"github.com/milvus-io/milvus/pkg/v2/proto/planpb"
)

func TestMockSelectivityEstimator(t *testing.T) {
	estimator := NewMockSelectivityEstimator()

	// Test nil expression
	selectivity := estimator.EstimateSelectivity(nil, nil)
	assert.Equal(t, 1.0, selectivity)

	// Test with schema
	schema := &schemapb.CollectionSchema{
		Fields: []*schemapb.FieldSchema{
			{FieldID: 100, Name: "price", DataType: schemapb.DataType_Double},
			{FieldID: 101, Name: "category", DataType: schemapb.DataType_Int64},
		},
	}

	// Test UnaryRangeExpr
	expr := &planpb.Expr{
		Expr: &planpb.Expr_UnaryRangeExpr{
			UnaryRangeExpr: &planpb.UnaryRangeExpr{
				ColumnInfo: &planpb.ColumnInfo{FieldId: 100},
				Op:         planpb.OpType_LessThan,
				Value:      &planpb.GenericValue{Val: &planpb.GenericValue_FloatVal{FloatVal: 10.0}},
			},
		},
	}

	selectivity = estimator.EstimateSelectivity(expr, schema)
	assert.Greater(t, selectivity, 0.0)
	assert.LessOrEqual(t, selectivity, 1.0)
}

func TestStatisticsBasedSelectivityEstimator_UnaryRange(t *testing.T) {
	accessor := NewCollectionStatisticsAccessor(0)
	collectionID := int64(1)

	// Set up test statistics
	accessor.SetFieldStatistics(collectionID, 100, &FieldStatistics{
		FieldID:  100,
		Min:      float64(0.0),
		Max:      float64(100.0),
		Cardinality: 1000,
		RowCount: 10000,
	})

	estimator := NewStatisticsBasedSelectivityEstimator(accessor, collectionID, nil)

	schema := &schemapb.CollectionSchema{
		Fields: []*schemapb.FieldSchema{
			{FieldID: 100, Name: "price", DataType: schemapb.DataType_Double},
		},
	}

	// Test LessThan
	expr := &planpb.Expr{
		Expr: &planpb.Expr_UnaryRangeExpr{
			UnaryRangeExpr: &planpb.UnaryRangeExpr{
				ColumnInfo: &planpb.ColumnInfo{FieldId: 100, DataType: schemapb.DataType_Double},
				Op:         planpb.OpType_LessThan,
				Value:      &planpb.GenericValue{Val: &planpb.GenericValue_FloatVal{FloatVal: 50.0}},
			},
		},
	}

	selectivity := estimator.EstimateSelectivity(expr, schema)
	assert.Greater(t, selectivity, 0.0)
	assert.Less(t, selectivity, 1.0)
	// For price < 50 with range [0, 100], selectivity should be approximately 0.5
	assert.InDelta(t, 0.5, selectivity, 0.1)
}

func TestStatisticsBasedSelectivityEstimator_BinaryRange(t *testing.T) {
	accessor := NewCollectionStatisticsAccessor(0)
	collectionID := int64(1)

	accessor.SetFieldStatistics(collectionID, 100, &FieldStatistics{
		FieldID:  100,
		Min:      float64(0.0),
		Max:      float64(100.0),
		Cardinality: 1000,
		RowCount: 10000,
	})

	estimator := NewStatisticsBasedSelectivityEstimator(accessor, collectionID, nil)

	schema := &schemapb.CollectionSchema{
		Fields: []*schemapb.FieldSchema{
			{FieldID: 100, Name: "price", DataType: schemapb.DataType_Double},
		},
	}

	// Test BETWEEN
	expr := &planpb.Expr{
		Expr: &planpb.Expr_BinaryRangeExpr{
			BinaryRangeExpr: &planpb.BinaryRangeExpr{
				ColumnInfo:     &planpb.ColumnInfo{FieldId: 100, DataType: schemapb.DataType_Double},
				LowerInclusive: true,
				UpperInclusive: true,
				LowerValue:      &planpb.GenericValue{Val: &planpb.GenericValue_FloatVal{FloatVal: 20.0}},
				UpperValue:      &planpb.GenericValue{Val: &planpb.GenericValue_FloatVal{FloatVal: 30.0}},
			},
		},
	}

	selectivity := estimator.EstimateSelectivity(expr, schema)
	assert.Greater(t, selectivity, 0.0)
	assert.Less(t, selectivity, 1.0)
	// For price BETWEEN 20 AND 30 with range [0, 100], selectivity should be approximately 0.1
	assert.InDelta(t, 0.1, selectivity, 0.05)
}

func TestStatisticsBasedSelectivityEstimator_Term(t *testing.T) {
	accessor := NewCollectionStatisticsAccessor(0)
	collectionID := int64(1)

	accessor.SetFieldStatistics(collectionID, 101, &FieldStatistics{
		FieldID:  101,
		Cardinality: 10, // 10 distinct values
		RowCount: 1000,
	})

	estimator := NewStatisticsBasedSelectivityEstimator(accessor, collectionID, nil)

	schema := &schemapb.CollectionSchema{
		Fields: []*schemapb.FieldSchema{
			{FieldID: 101, Name: "category", DataType: schemapb.DataType_Int64},
		},
	}

	// Test IN expression
	expr := &planpb.Expr{
		Expr: &planpb.Expr_TermExpr{
			TermExpr: &planpb.TermExpr{
				ColumnInfo: &planpb.ColumnInfo{FieldId: 101},
				Values: []*planpb.GenericValue{
					{Val: &planpb.GenericValue_Int64Val{Int64Val: 1}},
					{Val: &planpb.GenericValue_Int64Val{Int64Val: 2}},
				},
			},
		},
	}

	selectivity := estimator.EstimateSelectivity(expr, schema)
	assert.Greater(t, selectivity, 0.0)
	assert.LessOrEqual(t, selectivity, 1.0)
	// For 2 values out of 10 distinct values, selectivity should be approximately 0.2
	assert.InDelta(t, 0.2, selectivity, 0.1)
}

func TestStatisticsBasedSelectivityEstimator_BinaryExpr(t *testing.T) {
	accessor := NewCollectionStatisticsAccessor(0)
	collectionID := int64(1)

	accessor.SetFieldStatistics(collectionID, 100, &FieldStatistics{
		FieldID:  100,
		Min:      float64(0.0),
		Max:      float64(100.0),
		Cardinality: 1000,
		RowCount: 10000,
	})

	estimator := NewStatisticsBasedSelectivityEstimator(accessor, collectionID, nil)

	schema := &schemapb.CollectionSchema{
		Fields: []*schemapb.FieldSchema{
			{FieldID: 100, Name: "price", DataType: schemapb.DataType_Double},
		},
	}

	// Test AND expression
	leftExpr := &planpb.Expr{
		Expr: &planpb.Expr_UnaryRangeExpr{
			UnaryRangeExpr: &planpb.UnaryRangeExpr{
				ColumnInfo: &planpb.ColumnInfo{FieldId: 100, DataType: schemapb.DataType_Double},
				Op:         planpb.OpType_LessThan,
				Value:      &planpb.GenericValue{Val: &planpb.GenericValue_FloatVal{FloatVal: 50.0}},
			},
		},
	}

	rightExpr := &planpb.Expr{
		Expr: &planpb.Expr_UnaryRangeExpr{
			UnaryRangeExpr: &planpb.UnaryRangeExpr{
				ColumnInfo: &planpb.ColumnInfo{FieldId: 100, DataType: schemapb.DataType_Double},
				Op:         planpb.OpType_GreaterThan,
				Value:      &planpb.GenericValue{Val: &planpb.GenericValue_FloatVal{FloatVal: 10.0}},
			},
		},
	}

	expr := &planpb.Expr{
		Expr: &planpb.Expr_BinaryExpr{
			BinaryExpr: &planpb.BinaryExpr{
				Op:    planpb.BinaryExpr_LogicalAnd,
				Left:  leftExpr,
				Right: rightExpr,
			},
		},
	}

	selectivity := estimator.EstimateSelectivity(expr, schema)
	assert.Greater(t, selectivity, 0.0)
	assert.Less(t, selectivity, 1.0)
	// AND should have lower selectivity than either operand alone
	leftSelectivity := estimator.EstimateSelectivity(leftExpr, schema)
	rightSelectivity := estimator.EstimateSelectivity(rightExpr, schema)
	assert.Less(t, selectivity, leftSelectivity)
	assert.Less(t, selectivity, rightSelectivity)
}

func TestStatisticsBasedSelectivityEstimator_Fallback(t *testing.T) {
	// Test with no statistics available - should fall back to mock estimator
	accessor := NewCollectionStatisticsAccessor(0)
	collectionID := int64(1)

	fallbackEstimator := NewMockSelectivityEstimator()
	estimator := NewStatisticsBasedSelectivityEstimator(accessor, collectionID, fallbackEstimator)

	schema := &schemapb.CollectionSchema{
		Fields: []*schemapb.FieldSchema{
			{FieldID: 100, Name: "price", DataType: schemapb.DataType_Double},
		},
	}

	expr := &planpb.Expr{
		Expr: &planpb.Expr_UnaryRangeExpr{
			UnaryRangeExpr: &planpb.UnaryRangeExpr{
				ColumnInfo: &planpb.ColumnInfo{FieldId: 100, DataType: schemapb.DataType_Double},
				Op:         planpb.OpType_LessThan,
				Value:      &planpb.GenericValue{Val: &planpb.GenericValue_FloatVal{FloatVal: 10.0}},
			},
		},
	}

	// Should not panic and should return a valid selectivity
	selectivity := estimator.EstimateSelectivity(expr, schema)
	assert.Greater(t, selectivity, 0.0)
	assert.LessOrEqual(t, selectivity, 1.0)
}

func TestDecideFilterStrategy(t *testing.T) {
	// Test threshold-based decision
	assert.False(t, DecideFilterStrategy(0.01, DefaultSelectivityThreshold)) // Low selectivity -> standard filtering
	assert.True(t, DecideFilterStrategy(0.1, DefaultSelectivityThreshold))     // High selectivity -> iterative filtering
	assert.True(t, DecideFilterStrategy(0.05, DefaultSelectivityThreshold))   // At threshold -> iterative filtering
}

func TestStatisticsAccessor(t *testing.T) {
	accessor := NewCollectionStatisticsAccessor(0)
	collectionID := int64(1)
	fieldID := int64(100)

	// Test setting and getting statistics
	stats := &FieldStatistics{
		FieldID:   fieldID,
		Min:       float64(0.0),
		Max:       float64(100.0),
		Cardinality: 1000,
		RowCount: 10000,
	}

	accessor.SetFieldStatistics(collectionID, fieldID, stats)

	ctx := context.Background()
	retrievedStats, err := accessor.GetFieldStatistics(ctx, collectionID, fieldID)
	assert.NoError(t, err)
	assert.NotNil(t, retrievedStats)
	assert.Equal(t, stats.FieldID, retrievedStats.FieldID)

	// Test cache clear
	accessor.ClearCache()
	retrievedStats, err = accessor.GetFieldStatistics(ctx, collectionID, fieldID)
	// After clear, should return nil (no statistics available)
	assert.Nil(t, retrievedStats)
}

