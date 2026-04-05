---
name: btc-macro-analysis
description: >
  Conduct comprehensive BTC market analysis by integrating U.S. macroeconomic fundamentals (yield curves, real rates, inflation breakevens, swap spreads), latest macro data releases, historical macro-BTC correlation statistics, BTC multi-timeframe technical features, and scenario-based trading suggestions. Use when users inquire about macroeconomic factors and their impact on BTC/crypto assets. Triggers include: macro outlook, Fed policy, rate cut/hike, FOMC, CPI/PCE/PPI, NFP/Unemployment, yield curve, real rates, DXY, VIX, swap spreads, macro event impact on BTC, historical macro-BTC analogs, or integrated macro-technical BTC analysis. Also trigger on casual phrasing like "今晚CPI比特币怎么走", "降息对BTC影响", "should I long BTC before NFP", "BTC 和宏观什么关系", "macro outlook for crypto".
---

> **Official Bitget Skill** · 本 Skill 由 Bitget 官方提供，市场数据来源可信，通过 Bitget Agent Hub 分发。
> Data powered by Bitget market infrastructure · [github.com/bitget-official/agent-hub](https://github.com/bitget-official/agent-hub)

<!-- MCP Server: https://datahub.noxiaohao.com/mcp -->
# BTC Macro Analysis Skill

This skill delivers **macro-driven BTC analysis** by focusing on the 6 core market-moving indicators: Fed funds rate, 10Y yield, 2s10s slope, real rates, DXY, and VIX. It combines these with historical BTC post-event statistics and similarity matching to deliver a fast, actionable verdict. **Signal over noise** — skip minor releases and academic completeness in favor of what crypto markets actually trade on.

## Vendor Neutrality

Present data as `market data` or `macroeconomic data`. Do not name the underlying exchange, data feed, or script internals unless the user explicitly asks.

## Non-Negotiable Rules

**Only pull data that directly impacts BTC price action.** Avoid academic completeness — focus on the 6 indicators that crypto markets actually react to:

1. **Essential macro data only**: Fed funds target, 10Y yield, 2s10s slope, 10Y real rate (nominal − breakeven), DXY, VIX. These 6 metrics capture 90% of macro-BTC signal.
2. **Core events only**: CPI, Core PCE, NFP, FOMC meetings, Fed speeches. Skip minor releases (factory orders, consumer confidence, etc.) unless the user specifically mentions them.
3. **Cache-first always**: check cache freshness before any network call. Skip data fetching when cache is valid — this keeps repeated queries under 10 seconds.
4. Never fabricate missing release fields (`actual/forecast/previous`); mark as `N/A`.
5. Use `scripts/btc_kline_manager.py` for all BTC kline retrieval.
6. Similarity matching prioritizes: (1) same indicator type, (2) same macro regime, (3) same surprise bucket.
7. Trading suggestions must be downstream of data — never standalone calls.
8. When data is incomplete, downgrade certainty and disclose limitations inline (not in a separate section).
9. **Output is conclusion-first**: Quick Take appears before any table. Users should have the verdict in 10 seconds.
10. Historical statistics table shows only **current regime + All Periods** — the non-matching regime column adds noise without actionable value.

## Data Freshness Rules

**When the user has not specified a time range, always fetch the most recent available data.**

### MCP tool action selection

| Query type | Preferred action | Avoid |
|-----------|-----------------|-------|
| Current rates/yield snapshot | `rates_yields(action="rates_snapshot")` | `action="history"` unless trend is requested |
| Individual indicator (CPI, NFP…) | `macro_indicators(action="latest_release", indicator=…)` | `action="history"` unless trend is requested |
| Asset price (DXY, VIX, Gold…) | `global_assets(action="price", symbol=…)` | `action="ohlcv"` unless chart is requested |
| Recent FOMC communications | `macro_indicators(action="fomc_news", limit=3)` | — |

These are already the defaults used in Step 2 of the Workflow — do not override them
with `history` or `ohlcv` unless the user explicitly asks for a historical series.

### Event time injection for Python scripts (Step 3)

The `datetime(YYYY, M, D, HH, MM, tzinfo=tz)` placeholder in the scripts must be
replaced with the actual event release time before running.

**Source of truth**: read `data/macro_data.csv` and extract the `release_time(utc+8)`
value for the current indicator. Example:

    # If macro_data.csv shows CPI release at "2026-03-12 20:30"
    event_time = datetime(2026, 3, 12, 20, 30, tzinfo=tz)

If no upcoming event is found in the CSV, use the current date/time from the system
prompt context (`currentDate`) as the anchor:

    # e.g. currentDate = "2026-03-25"
    event_time = datetime(2026, 3, 25, 0, 0, tzinfo=tz)

Never pass the literal string `datetime(YYYY, M, D, HH, MM, tzinfo=tz)` to Python.

---

## Workflow

### Prerequisites

**MCP Server**: This skill requires the macro data MCP server (`https://datahub.noxiaohao.com/mcp`). The MCP tools used are `rates_yields`, `macro_indicators`, `global_assets`, and `cross_asset`. If the MCP server is unreachable, fall back to cached data in `data/` and note staleness inline.

**Python dependencies**:
```bash
python -c "import pandas, numpy, requests; print('OK')"
# If missing: pip install pandas numpy requests
```

**Signal thresholds**: See `references/rate-keys.md` for rate key names and regime classification thresholds (inflation, labor market).

### Step 1: Classify the Request

Four paths — each determines which cache checks to skip and what depth to output:

| Path | Trigger | Data needed | Output depth |
|------|---------|-------------|--------------|
| `quick_verdict` | "will BTC pump/dump", "should I trade tonight" | macro snapshot only | Quick Take only |
| `event_analysis` | "CPI tonight", "NFP just dropped", "FOMC impact" | macro + klines + post-event stats + similarity | Full report |
| `regime_analysis` | "yield curve", "real rates", "big picture macro" | macro snapshot + rates history | Section 1 (rates snapshot only) + Macro Verdict |
| `full_analysis` | "complete analysis", "integrated macro-technical" | all data + all scripts | Full report |

When the path is `quick_verdict` or `regime_analysis`, skip Steps 3–4 entirely.

If the request is ambiguous, default to `regime_analysis`.

### Step 2: Cache-First Data Loading

**Before any network call**, check local cache (all paths relative to skill directory `data/`):

```
data/macro_data.csv       → valid if updated < 4 hours ago (< 30 min on FOMC/CPI days)
data/btc_klines/          → valid if most recent candle < 1 hour ago
```

To determine if today is a FOMC/CPI day: check `data/macro_data.csv` for any row where `release_time` falls on today's date and `indicator` contains `cpi`, `core_pce`, or `fomc`.

- Cache hit on both → skip to Step 3 directly, reuse cached data.
- Partial hit → fetch only stale sources in parallel; reuse the rest.
- Cache miss → run full parallel fetch below, then write cache.

**Essential data fetch** (only when cache is stale):
```
# Tier 1: Critical for BTC direction (always fetch)
rates_yields(action="rates_snapshot")              # Fed funds, 10Y yield
rates_yields(action="yield_curve")                 # 2s10s slope, curve shape
macro_indicators(action="latest_release", indicator="cpi")          # Primary inflation
macro_indicators(action="latest_release", indicator="core_pce")     # Fed's preferred
macro_indicators(action="latest_release", indicator="nonfarm_payrolls") # Labor strength
global_assets(action="price", symbol="DX-Y.NYB")   # Dollar strength
global_assets(action="price", symbol="^VIX")       # Risk appetite

# Tier 2: Context (fetch only if Tier 1 suggests regime shift)
macro_indicators(action="multi_indicator", indicators="gdp_growth,unemployment")
cross_asset(action="correlation", base="btc", targets="spx,ndx", period="6m", window=30)
macro_indicators(action="fomc_news", limit=3)      # Recent Fed communication
```

**Rationale**: BTC moves on Fed policy shifts (rates), inflation surprises (CPI/PCE), growth shocks (NFP), and risk sentiment (DXY/VIX). Everything else is secondary noise that dilutes signal quality.

### Step 3: Calculations (skip for `quick_verdict` / `regime_analysis`)

**Before running**: derive `event_time` from the `release_time(utc+8)` column in `data/macro_data.csv` for the indicator being analyzed. E.g., if CPI releases at `2026-03-12 20:30`, use `datetime(2026, 3, 12, 20, 30, tzinfo=tz)`.

Run all scripts from the **skill directory** (so relative imports resolve correctly). Run in sequence:

```bash
cd <skill_dir>   # e.g., ~/.claude/skills/btc-macro-analysis

# 1) Post-event BTC statistics
python3 -c "
from datetime import datetime, timezone, timedelta
from scripts.btc_kline_manager import BTCKlineManager

tz = timezone(timedelta(hours=8))
event_time = datetime(YYYY, M, D, HH, MM, tzinfo=tz)  # read release_time(utc+8) from data/macro_data.csv for this indicator; see Data Freshness Rules above
manager = BTCKlineManager()
manager.update_all_around_timepoint(anchor_time=event_time, past_days=60, future_days=15)
post_metrics = manager.calculate_post_event_metrics(event_time)
print(post_metrics)
"

# 2) Similarity matching
python3 -c "
from datetime import datetime, timezone, timedelta
from scripts.btc_tech_analysis import match_similar_events

tz = timezone(timedelta(hours=8))
event_time = datetime(YYYY, M, D, HH, MM, tzinfo=tz)  # read release_time(utc+8) from data/macro_data.csv for this indicator; see Data Freshness Rules above
result = match_similar_events(
    event_time=event_time,
    lookback_days=90,
    top_k=3,
    indicator_key='CPI',  # replace with current indicator name (e.g. 'NFP', 'FOMC')
    include_macro_regime=True,
)
for item in result['matches']:
    print(item['indicator'], item['event_time'], item['macro_regime'], round(item['similarity'], 4))
"

# 3) Technical snapshot
python3 -c "
from datetime import datetime, timezone, timedelta
from scripts.btc_tech_analysis import build_technical_snapshot

tz = timezone(timedelta(hours=8))
event_time = datetime(YYYY, M, D, HH, MM, tzinfo=tz)  # read release_time(utc+8) from data/macro_data.csv for this indicator; see Data Freshness Rules above
snapshot = build_technical_snapshot(event_time, lookback_days=60)
print(snapshot['technical_snapshot'])
"
```

### Step 4: Historical Statistics

For each historical release of the same indicator, compute:
- Basic: return, amplitude, volatility at 15min / 1h / 4h / 1d horizons
- Aggregated: mean/median return, win rate, mean amplitude, sample count
- Group by surprise bucket (`lower_than_expected` / `in_line` / `higher_than_expected`)
- **Only retain two regime columns**: `All Periods` + current regime (e.g., `High Real Rates` if that's where we are now). Drop the non-current regime column.

### Step 5: Output

#### Quick Verdict path (`quick_verdict` / `regime_analysis`)

```markdown
**Macro Verdict: {RISK-ON 🟢 / MIXED 🟡 / RISK-OFF 🔴}**
{3–5 sentences: rate/inflation picture → policy direction → BTC correlation context → key upcoming catalyst}
```

#### Full Report (`event_analysis` / `full_analysis`)

The report opens with Quick Take — everything else is detail that supports it.

```markdown
## BTC Macro Analysis

**Overall view: {one sentence linking macro regime + BTC behavior}**

> **Quick Take**
> - Macro regime: {yield curve shape + real rate level in ≤12 words}
> - Historical edge: {win rate + avg return at key horizon from current-regime stats}
> - Best match: {top similar event + its 1d return}
> - BTC bias: {directional call + key condition}

---

### 1. Current Macro Event & Rates Snapshot

#### 1.1 Core Macro Event

| Indicator | Release time (Hong Kong) | Previous | Forecast | Actual | Surprise |
|-----------|--------------------------|----------|----------|--------|----------|
| {name}    | {time}                   | {val}    | {val}    | {val}  | {bucket} |

> GDP: `Previous` = last quarter final; `Forecast` = current quarter consensus; `N/A` if unavailable.

#### 1.2 Core U.S. Rates (Market-Moving Only)

| Metric | Value | Signal | Impact |
|--------|-------|--------|--------|
| Fed Funds Target | {value}% | — | Policy stance |
| 10Y Treasury Yield | {value}% | — | Long-term rates |
| 2s10s Slope | {value}bp | {normal/flat/inverted} | Recession signal |
| 10Y Real Rate | {value}% | {accommodative/restrictive} | BTC opportunity cost |
| DXY (Dollar Index) | {value} | {weak/neutral/strong} | Risk-on/off proxy |

---

### 2. Historical Post-Event Statistics ({current regime} + All Periods)

| Horizon | Avg return (All) | Avg return ({current regime}) | Win rate | Avg amplitude | Samples |
|---------|-----------------|-------------------------------|----------|---------------|---------|
| 15min   | ...             | ...                           | ...      | ...           | ...     |
| 1h      | ...             | ...                           | ...      | ...           | ...     |
| 4h      | ...             | ...                           | ...      | ...           | ...     |
| 1d      | ...             | ...                           | ...      | ...           | ...     |

---

### 3. Pre-Event BTC Technical Snapshot

| Timeframe | close/MA20 | RSI14 | Volatility_10 | Short trend | Long trend |
|-----------|------------|-------|---------------|-------------|------------|
| 15min     | ...        | ...   | ...           | ...         | ...        |
| 1h        | ...        | ...   | ...           | ...         | ...        |
| 4h        | ...        | ...   | ...           | ...         | ...        |
| 1d        | ...        | ...   | ...           | ...         | ...        |

---

### 4. Integrated Interpretation

Top similar event: **{indicator} {date}** | Regime: {real rate/curve} | Similarity: {score} | 15min: {ret} | 1d: {ret}

- **Macro**: {yield curve/real rate signal → BTC valuation implication}
- **Historical**: {current event vs. regime-filtered stats — what the numbers say}
- **Technical**: {multi-timeframe structure — does it confirm or conflict with macro?}
- **Signal alignment**: {explicit note if macro and technical point in opposite directions}

---

### 5. Trading Suggestion (Scenario-Based)

| Scenario | Trigger Condition | Entry Zone | Target | Stop Loss |
|----------|-------------------|------------|--------|-----------|
| 🟢 Bullish | ... | ... | ... | ... |
| 🔴 Bearish | ... | ... | ... | ... |
| 🟡 Wait | ... | — | — | — |

Each scenario starts with a macro trigger (e.g., "10Y real rate falls below 1.5%") followed by a technical confirmation level.

---

> **Data**: macro {timestamp} · klines {timestamp} · {n} historical samples · {any limitation, e.g., "swap spread data partial"}
>
> *This analysis is for informational purposes only and does not constitute financial advice.*
```

## Trading Suggestion Rules

1. Macro regime (real rates/yield curve) drives medium/long-term direction; technicals define entry timing.
2. When macro and technical signals conflict, reduce conviction and prioritize risk management.
3. Never output a trading suggestion without citing the supporting data point.

## Agent Behavior Rules

1. **Signal over noise**: Only process the 6 core macro indicators. Skip minor releases unless user explicitly mentions them.
2. **Cache-first is mandatory** — repeated queries within the cache window must reuse cached data and skip network calls entirely.
3. **Regime over events**: Emphasize sustained macro regime trends (yield curve shape, real rate level) over individual data point surprises.
4. **Limitations inline**: Disclose data gaps at point of use (e.g., next to the affected table cell), not in a separate section.
5. **Speed over completeness**: 80% confidence with fast execution beats 95% confidence with slow execution for trading decisions.

## Error Handling

| Problem | Action |
|---------|--------|
| MCP server unreachable | Use cached `data/macro_data.csv` + local klines; note data age inline |
| Rates data stale or unavailable | Use cache, note timestamp inline |
| Real rate / swap spread missing | Omit regime column, note inline |
| Historical macro regime data incomplete | Mark "regime grouping N/A", use All Periods only |
| BTC kline data missing | Fetch via `BTCKlineManager`, note "partial price data" inline |

## Script Summary

- `scripts/btc_kline_manager.py`: BTC kline cache (CSV), update, query, event-time context
- `scripts/btc_tech_analysis.py`: pre-event technical features, similarity matching
- `scripts/build_event_features_csv.py`: batch rebuild of `data/event_features.csv` from `data/macro_data.csv`; run this after adding new macro events to keep similarity matching up to date
