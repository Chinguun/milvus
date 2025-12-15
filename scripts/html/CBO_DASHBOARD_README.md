# CBO Dashboard - HTML Visualization Interface

A standalone HTML dashboard for visualizing Cost-Based Optimizer (CBO) evaluation results in Milvus.

## Features

- **Real-time Metrics**: Fetches and displays CBO metrics from Milvus HTTP API
- **Interactive Charts**: Visual representation of strategy distribution, selectivity, and performance
- **Query Table**: Detailed table of all query metrics with filtering and search
- **Statistics Cards**: Quick overview of key metrics (total queries, improved/degraded counts, etc.)
- **Evaluation Summary**: Aggregated statistics and performance comparison
- **Recommendations**: CBO optimization recommendations
- **Auto-refresh**: Automatically refreshes data every 30 seconds

## Usage

### Option 1: Open Directly in Browser

1. Make sure Milvus is running and accessible
2. Open `scripts/html/cbo_dashboard.html` in your web browser (Chrome, Firefox, Safari, Edge)
3. The dashboard will automatically connect to `http://localhost:9091` by default
4. Click "🔄 Refresh Data" to load CBO metrics

### Option 2: Serve via HTTP Server

If you encounter CORS issues when opening the file directly, serve it via a local HTTP server:

```bash
# Navigate to the html directory
cd scripts/html

# Using Python 3
python3 -m http.server 8000

# Using Python 2
python -m SimpleHTTPServer 8000

# Using Node.js (if you have http-server installed)
npx http-server -p 8000
```

Then open: `http://localhost:8000/cbo_dashboard.html`

### Option 3: Access from Builder Container

If running inside the builder container, you can:

1. Copy the HTML file to a location accessible from your host
2. Open it in your browser on the host machine
3. Update the "Milvus HTTP Host" to point to your Milvus instance

## Configuration

The dashboard allows you to configure:

- **Milvus HTTP Host**: Default is `http://localhost:9091`
  - For containerized Milvus, use the host's IP address and port
  - Example: `http://192.168.1.100:9091`

- **Collection ID**: Optional filter to show metrics for a specific collection
  - Leave empty to show metrics from all collections
  - You can find collection IDs from the metrics table

## Dashboard Sections

### 1. Statistics Cards
Quick overview showing:
- Total queries
- Improved/Degraded/Neutral counts
- Strategy distribution (Standard vs Iterative)
- Average selectivity and execution time

### 2. Strategy Distribution Chart
Doughnut chart showing the proportion of queries using:
- Standard Filter (BitSet)
- Iterative Filter

### 3. Selectivity Distribution Chart
Bar chart showing the distribution of queries across selectivity ranges:
- 0-0.05 (very selective)
- 0.05-0.1
- 0.1-0.3
- 0.3-0.5
- 0.5-1.0 (not selective)

### 4. Performance Chart
Line chart showing execution times for each query

### 5. Query Metrics Table
Detailed table with:
- Query ID
- Filter Expression
- Selectivity Estimate
- Selected Strategy
- Execution Time
- Optimization Result
- Timestamp

**Filtering Options:**
- Filter by strategy (Standard/Iterative)
- Filter by result (Improved/Degraded/Neutral)
- Search by filter expression

### 6. Evaluation Summary
Aggregated statistics including:
- Total queries and breakdown
- Average time reduction
- Strategy distribution
- Performance comparison (if baseline data available)

### 7. Recommendations
CBO optimization recommendations based on the collected metrics

## Troubleshooting

### "No CBO metrics found"
- Ensure CBO evaluation is enabled: `proxy.cbo.evaluation.enabled=true`
- Make sure queries have been executed
- Verify Milvus HTTP endpoint is accessible

### CORS Errors
If you see CORS errors when opening the file directly:
- Use a local HTTP server (see Option 2 above)
- Or access Milvus via the same origin

### Connection Errors
- Verify Milvus is running: `curl http://localhost:9091/healthz`
- Check the HTTP host and port configuration
- For containerized setups, ensure ports are properly exposed

### Charts Not Displaying
- Check browser console for JavaScript errors
- Ensure Chart.js CDN is accessible
- Try refreshing the page

## Notes

- The dashboard uses Chart.js for visualizations (loaded from CDN)
- Data is fetched from Milvus REST API endpoints
- All time values are converted from nanoseconds to seconds for readability
- The dashboard auto-refreshes every 30 seconds when data is loaded
- No data is stored locally - all data is fetched from Milvus in real-time

## Browser Compatibility

Tested and working on:
- Chrome/Chromium (recommended)
- Firefox
- Safari
- Edge

Requires modern browser with JavaScript enabled.
