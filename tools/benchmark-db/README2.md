# SQLite Benchmark Results

This document summarizes the performance benchmarks for various SQLite configurations and queries. Key insights and detailed results are provided below.

---

## Benchmark Setup
The following setup was used for all benchmarks:
```sql
-- Schema
CREATE TABLE files(filename TEXT NOT NULL, fullpath TEXT NOT NULL UNIQUE, event_id INTEGER, object_type INTEGER);

-- Query
SELECT fullpath, object_type FROM files 
WHERE filename LIKE ? COLLATE BINARY 
ORDER BY filename ASC 
LIMIT ?;

-- PRAGMA Settings (unless noted otherwise)
PRAGMA journal_mode=WAL;
PRAGMA synchronous=OFF;
```

---

## 1. No Index
**Configuration**: No indexes applied.  
**Notes**: Baseline performance without optimizations.

| Date                  | Search  | Min | Max | Average | Median | StdDev | Notes           |
|-----------------------|---------|-----|-----|---------|--------|--------|-----------------|
| 2025/02/09 10:08:26   | Overall | 104 | 142 | 111     | 109    | 6      | 400 iterations |
| 2025/02/09 10:09:12   | Overall | 106 | 201 | 114     | 111    | 9      | 400 iterations |
| 2025/02/09 10:09:59   | Overall | 108 | 173 | 117     | 114    | 9      | 400 iterations |

**Key Insight**:  
- High latency (100+ ms) observed due to full table scans.

---

## 2. Single-Column Index (`idx_filename`)
**Configuration**:  
```sql
CREATE INDEX idx_filename ON files(filename COLLATE BINARY);
```  
**Notes**: Most significant performance improvement.

| Date                  | Search  | Min | Max | Average | Median | StdDev | Notes           |
|-----------------------|---------|-----|-----|---------|--------|--------|-----------------|
| 2025/02/09 10:12:19   | Overall | 13  | 111 | 55      | 68     | 24     | 400 iterations |
| 2025/02/09 10:12:41   | Overall | 14  | 76  | 55      | 67     | 23     | 400 iterations |
| 2025/02/09 10:13:04   | Overall | 14  | 138 | 56      | 68     | 24     | 400 iterations |
| 2025/02/09 10:17:28   | Overall | 13  | 608 | 61      | 68     | 41     | 1200 iterations|
| 2025/02/09 10:18:35   | Overall | 14  | 83  | 55      | 68     | 23     | 1200 iterations|
| 2025/02/09 10:19:43   | Overall | 14  | 122 | 56      | 67     | 24     | 1200 iterations|

**Key Insights**:  
- **~4x faster** than no-index baseline (Min: 13 ms vs. 104 ms).  
- Higher standard deviation due to occasional spikes (e.g., 608 ms).

---

## 3. Composite Index (`idx_filename_fullpath_obj_type`)
**Configuration**:  
```sql
CREATE INDEX idx_filename_fullpath_obj_type ON files(filename COLLATE BINARY, fullpath, object_type);
```  
**Notes**: Performance degraded compared to the single-column index.

| Date                  | Search  | Min | Max | Average | Median | StdDev | Notes           |
|-----------------------|---------|-----|-----|---------|--------|--------|-----------------|
| 2025/02/09 10:28:47   | Overall | 16  | 1093| 100     | 113    | 0      | 1200 iterations|
| 2025/02/09 10:30:35   | Overall | 16  | 188 | 90      | 114    | 42     | 1200 iterations|
| 2025/02/09 10:32:47   | Overall | 16  | 2669| 109     | 116    | 0      | 1200 iterations|

**Key Insights**:  
- Composite index increased latency (Average: 90–109 ms vs. 55–61 ms).  
- Likely caused by larger index size and suboptimal query plan selection.

---

## 4. Disabled `journal_mode=WAL` and `synchronous=OFF`
**Configuration**: Default journal mode and synchronous settings.  

| Date                  | Search  | Min | Max | Average | Median | StdDev | Notes           |
|-----------------------|---------|-----|-----|---------|--------|--------|-----------------|
| 2025/02/09 10:44:20   | Overall | 13  | 346 | 55      | 67     | 25     | 1200 iterations|
| 2025/02/09 10:45:27   | Overall | 14  | 79  | 55      | 67     | 23     | 1200 iterations|
| 2025/02/09 10:46:34   | Overall | 14  | 133 | 55      | 67     | 23     | 1200 iterations|

**Key Insight**:  
- Minimal impact on read-only queries, confirming `WAL`/`synchronous` primarily affect writes.

---

## 5. Adjusted `maxSearchResults`
**Configuration**: Reduced result limits (`maxSearchResults = 2000` and `1000`).  

| Date                  | Search  | Min | Max | Average | Median | StdDev | Notes           |
|-----------------------|---------|-----|-----|---------|--------|--------|-----------------|
| 2025/02/09 11:12:30   | Overall | 10  | 927 | 49      | 68     | 40     | 1200 iterations|
| 2025/02/09 11:13:27   | Overall | 10  | 140 | 47      | 66     | 24     | 1200 iterations|
| 2025/02/09 11:14:23   | Overall | 10  | 93  | 46      | 61     | 24     | 1200 iterations|
| ... (additional rows omitted for brevity) | | | | | | | |

**Key Insights**:  
- Lower `maxSearchResults` reduced average latency slightly (46–49 ms).  
- High standard deviation persisted due to outlier spikes.

---

## 6. Optimized Query Parameter (`"%"+prefix+"%"`)
**Configuration**: Wildcard search pattern adjusted.  

| Date                  | Search  | Min | Max | Average | Median | StdDev | Notes           |
|-----------------------|---------|-----|-----|---------|--------|--------|-----------------|
| 2025/02/09 13:23:58   | Overall | 1   | 122 | 34      | 19     | 42     | 1200 iterations|
| 2025/02/09 13:24:39   | Overall | 1   | 134 | 34      | 14     | 42     | 1200 iterations|
| 2025/02/09 13:25:20   | Overall | 1   | 152 | 34      | 15     | 42     | 1200 iterations|

**Key Insights**:  
- **Dramatic improvement**: Min latency dropped to **1 ms** (from 10–14 ms).  
- Wildcard placement optimized index usage, reducing scan overhead.

---

## Summary of Findings
1. **Indexes Matter**: Adding `idx_filename` reduced latency by ~4x.  
2. **Avoid Over-Indexing**: Composite indexes can degrade performance if unused columns are included.  
3. **Query Tuning**: Adjusting wildcard placement (`"%"+prefix+"%"`) yielded the largest gains (1 ms min).  
4. **PRAGMA Settings**: `journal_mode=WAL` and `synchronous=OFF` had minimal impact on read-heavy workloads.