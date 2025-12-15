# CBO Debugging Guide

## Issue
CBO logs are not appearing after running the demo script multiple times.

## Changes Made
Added comprehensive Info-level logging to trace CBO execution:

1. **Entry point logging**: Logs when CBO check happens with DSL string, hints, and plan status
2. **Function entry logging**: Logs when `applyCostBasedOptimization` is called
3. **Filter extraction logging**: Logs each step of extracting the filter expression from the plan
4. **Early exit logging**: Logs if CBO is skipped (no filter expression, hints already set, etc.)

## How to Debug

### Step 1: Rebuild Milvus
```bash
cd /home/chinguun/development/code/milvus
docker exec milvus-builder-1 bash -c "cd /go/src/github.com/milvus-io/milvus && source scripts/setenv.sh && go build -o bin/milvus ./cmd/roles/..."
```

### Step 2: Restart Milvus
```bash
# Stop current Milvus
docker exec milvus-builder-1 pkill -f milvus

# Start Milvus (inside container)
docker exec -d milvus-builder-1 bash -c "cd /go/src/github.com/milvus-io/milvus && ./bin/milvus run standalone"
```

### Step 3: Run Demo Script
```bash
python3 query_processing_demo/demo_script.py
```

### Step 4: Check Logs
Look for these log messages in the Milvus logs:

1. **CBO Check**: `"CBO: Checking if CBO should be applied"`
   - Should show: collection name, DSL string (filter expression), hints, plan status

2. **CBO Function Call**: `"CBO: applyCostBasedOptimization called"`
   - Should show: collection name, whether plan is nil

3. **Filter Extraction**: `"CBO: Extracting filter expression"` and `"CBO: Got predicates"`
   - Should show: whether vectorAnns is nil, whether predicates is nil

4. **CBO Decision**: `"🔥🔥🔥 CBO Decision: ..."`
   - This is the final decision log

### Step 5: Search Logs
```bash
# Search for all CBO-related logs
docker exec milvus-builder-1 grep -i "CBO" /go/src/github.com/milvus-io/milvus/logs/*.log | tail -50

# Or if logs are in /tmp
docker exec milvus-builder-1 grep -i "CBO" /tmp/standalone.log | tail -50

# Check for CBO log file
docker exec milvus-builder-1 ls -la /go/src/github.com/milvus-io/milvus/logs/cbo.log 2>/dev/null
```

## Possible Issues to Check

1. **No CBO check log**: CBO function is not being called
   - Check if `tryGeneratePlan` is being executed
   - Check if hints are already set before CBO runs

2. **"Plan is nil"**: Plan creation failed
   - Check plan creation errors in logs

3. **"vectorAnns is nil"**: Plan structure is unexpected
   - Check plan structure - might be using different plan type

4. **"predicates is nil"**: Filter expression not in plan
   - Check if DSL string contains the filter expression
   - Check if `CreateSearchPlan` properly parses the DSL

5. **"No filter expression found"**: Filter not extracted
   - Verify the DSL string contains the filter (e.g., "price < 10")
   - Check if the filter is being parsed correctly

6. **"Skipped because hints are already set"**: Hints are set before CBO runs
   - Check where hints are being set
   - Might need to check earlier in the code path

## Expected Log Flow

When working correctly, you should see:
```
INFO: CBO: Checking if CBO should be applied {collection: "cbo_demo", dsl: "price < 10", hints: "", plan_is_nil: false}
INFO: CBO: applyCostBasedOptimization called {collection: "cbo_demo", plan_is_nil: false}
INFO: CBO: Extracting filter expression {vectorAnns_is_nil: false}
INFO: CBO: Got predicates {predicates_is_nil: false}
INFO: 🔥🔥🔥 CBO Decision: Standard filtering selected 🔥🔥🔥 {selectivity: 0.001, threshold: 0.05, ...}
```

## Next Steps

After running the demo script with the new logging:
1. Check which log messages appear
2. Identify where the flow stops
3. Report findings for further investigation

