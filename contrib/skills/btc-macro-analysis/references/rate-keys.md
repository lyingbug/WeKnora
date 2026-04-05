# Rate Keys & Indicator Reference

## Available Rate Keys (`rates_yields`)

| Key | Description |
|-----|-------------|
| `fed_funds` | Effective Federal Funds Rate |
| `fed_funds_target_upper` | Fed Funds target upper bound |
| `fed_funds_target_lower` | Fed Funds target lower bound |
| `prime_rate` | US Prime Rate |
| `mortgage_30y` | 30-Year mortgage rate |
| `tips_10y` | 10-Year TIPS yield (real rate) |

## Available Macro Indicators (`macro_indicators`)

| Indicator | Description |
|-----------|-------------|
| `cpi` | Consumer Price Index (YoY%) |
| `core_pce` | Core PCE (Fed's preferred inflation measure) |
| `nonfarm_payrolls` | Non-Farm Payrolls (monthly jobs added) |
| `unemployment` | Unemployment rate (%) |
| `industrial_production` | Industrial Production index |
| `ppi` | Producer Price Index |
| `consumer_sentiment` | University of Michigan Consumer Sentiment |
| `ism_manufacturing` | ISM Manufacturing PMI |
| `initial_claims` | Initial jobless claims (weekly) |

## Signal Thresholds

### Inflation Regime
- Core PCE < 2%: Below target → dovish pressure
- Core PCE 2–2.5%: On target → neutral
- Core PCE > 2.5%: Above target → hawkish pressure
- Core PCE > 3%: Significantly above → active tightening

### Labor Market
- Unemployment < 4%: Tight labor market → wage pressure → hawkish
- Unemployment > 5%: Slack → dovish room
- NFP < 100k: Weak → dovish signal
- NFP > 250k: Strong → hawkish signal
