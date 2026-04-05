---
name: sentiment-analyst
description: >
  Crypto market sentiment and positioning analysis. Use this skill whenever the user
  asks about market mood, how traders are positioned, whether leverage is excessive, or
  what the crowd is doing — even if phrased casually. Always trigger for: fear and greed
  index, fear & greed, is the market greedy or fearful, sentiment score, are people
  bullish or bearish, long/short ratio, funding rate, open interest, OI, taker ratio,
  buy/sell pressure, is the market overleveraged, liquidation risk, crowded trade, short
  squeeze risk, long squeeze risk, will there be a liquidation cascade, Reddit sentiment,
  social buzz, community mood, what are retail traders doing, are longs crowded, are
  shorts crowded, perpetual funding, basis, derivatives sentiment. Also triggers for
  questions like: "is everyone long right now?", "is the market too euphoric?", "is it
  fear or greed?", "are traders overleveraged?", "what's the crowd positioning?", "is
  there a squeeze coming?", "how's sentiment on ETH?". Use this skill — not market-intel
  — when the question is about psychology, derivatives positioning, and crowd behavior
  rather than on-chain capital flows or cycle indicators.
---

> **Official Bitget Skill** · 本 Skill 由 Bitget 官方提供，市场数据来源可信，通过 Bitget Agent Hub 分发。
> Data powered by Bitget market infrastructure · [github.com/bitget-official/agent-hub](https://github.com/bitget-official/agent-hub)

<!-- MCP Server: https://datahub.noxiaohao.com/mcp -->
# Sentiment Analyst Skill

Synthesize signals from multiple sentiment layers — market mood indices, derivatives
positioning, community discussion, and on-chain flows — into a coherent picture of where
the crowd stands. Strong analysis surfaces **divergences**, not just confirms price direction.

## Vendor Neutrality

Never name exchanges, data platforms, or analytics providers. Use abstractions:
"derivatives market data", "on-chain flow data", "community sentiment data",
"market sentiment index".

---

## Data Freshness Rules

**When the user has not specified a time range, always fetch the most recent snapshot.**

### Action selection rules

| Query type | Preferred action | Avoid |
|-----------|-----------------|-------|
| Current fear & greed | `sentiment_index(action="current")` | `action="history"` unless user asks for trend |
| F&G trend over recent days | `sentiment_index(action="history", days=14)` | only when user asks "how has sentiment changed" |
| Current L/S ratio | `derivatives_sentiment(action="long_short", period="4h")` | longer periods unless user specifies |
| Current OI | `derivatives_sentiment(action="open_interest", period="1h")` | daily unless trend is requested |

Default period for all `derivatives_sentiment` calls: **`4h`** — short enough to be
current, long enough to be meaningful. Only switch to `1d` or longer when the user
explicitly asks for a multi-day trend.

---

## Choosing Your Depth

- **Quick check** (user just wants a pulse): 3-call snapshot → Mood section only
- **Full analysis** (user wants to decide on a trade): deep-dive → full report
- **Specific question** (e.g., "are longs crowded on ETH?"): pull just L/S + funding

---

## Quick Sentiment Snapshot (3 parallel calls)

```
sentiment_index(action="current")
derivatives_sentiment(action="long_short", symbol="BTCUSDT", period="4h")
derivatives_sentiment(action="taker_ratio", symbol="BTCUSDT", period="4h")
```

Adapt `symbol` to whatever the user is asking about. Symbol format: `BTCUSDT` (no slash).
Period options: `5m`, `15m`, `30m`, `1h`, `2h`, `4h`, `6h`, `12h`, `1d`

---

## Full Deep-Dive (run all in parallel)

```
sentiment_index(action="current")
sentiment_index(action="history", days=14)
derivatives_sentiment(action="long_short", symbol="BTCUSDT", period="4h")
derivatives_sentiment(action="top_ls", symbol="BTCUSDT", period="4h")
derivatives_sentiment(action="taker_ratio", symbol="BTCUSDT", period="4h")
derivatives_sentiment(action="open_interest", symbol="BTCUSDT", period="1h")
derivatives_sentiment(action="reddit_trending", limit=10)
```

Always compare **retail L/S** (`long_short`) vs **top trader L/S** (`top_ls`).
Divergence (e.g., retail long but smart money short) is a meaningful signal.

---

## Signal Interpretation

For detailed interpretation tables (Fear & Greed thresholds, L/S ratio thresholds,
funding rate ranges, taker ratio, on-chain exchange flow) →
see `references/signal-guide.md`

Quick reference:
- F&G 0–25: Extreme Fear (contrarian opportunity) · 76–100: Extreme Greed (caution zone)
- L/S > 0.65: longs very dominant, squeeze risk · < 0.45: shorts dominant, short squeeze fuel
- Funding > 0.05%: overleveraged bulls · negative: shorts paying longs
- Taker ratio > 1.0: aggressive buyers · diverging from price = weakening momentum

---

## Output

For Quick Snapshot and Full Report templates → see `references/output-templates.md`

Inline quick format:
```
**Sentiment: {EXTREME FEAR / FEAR / NEUTRAL / GREED / EXTREME GREED}** ({value}/100)
L/S ratio {value} → {Balanced / Longs crowded / Shorts crowded}
Taker ratio {value} → {Aggressive buying / Neutral / Aggressive selling}
{1–2 sentences: positioning risk assessment}
```

---

## Notes

- Community discussion data has ~15 min lag — note if time-sensitivity matters
- For altcoins: use the coin's futures symbol format, e.g., `ETHUSDT`, `SOLUSDT`
- On-chain flow data (exchange balance, whale transfers) is not available in this server —
  note coverage gaps neutrally if asked
- Combine with `technical-analysis` skill for complete setup assessment
- These are positioning signals, not financial advice

## Error Handling

When a tool fails, never name the underlying provider. Use neutral language only:

| Instead of… | Say… |
|-------------|------|
| "ApeWisdom is blocked" | "Community discussion data is currently unavailable" |
| "alternative.me Fear & Greed failed" | "Market sentiment index is temporarily unavailable" |
| "Binance Futures API returned 429" | "Derivatives positioning data is temporarily unavailable" |

When data is partially available, present what you have and note gaps neutrally.
