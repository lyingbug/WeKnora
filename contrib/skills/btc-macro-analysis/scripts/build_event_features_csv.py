#!/usr/bin/env python3
"""
基于 macro_data.csv 和本地 BTC K 线缓存，批量构建历史事件特征并导出 CSV。
"""

import argparse
import json
import os
from datetime import timedelta, timezone
from typing import Any, Dict, List, Optional

import pandas as pd

from btc_kline_manager import BTCKlineManager
from btc_tech_analysis import build_event_feature_sample

BEIJING_TZ = timezone(timedelta(hours=8))
SCRIPT_DIR = os.path.dirname(__file__)
DEFAULT_MACRO_CSV = os.path.join(SCRIPT_DIR, "..", "data", "macro_data.csv")
DEFAULT_OUTPUT_CSV = os.path.join(SCRIPT_DIR, "..", "data", "event_features.csv")


def _to_beijing_datetime(value: Any) -> Optional[pd.Timestamp]:
    ts = pd.to_datetime(value, errors="coerce")
    if pd.isna(ts):
        return None
    if ts.tzinfo is None:
        return ts.tz_localize(BEIJING_TZ)
    return ts.tz_convert(BEIJING_TZ)


def _infer_indicator_key(indicator: Any) -> str:
    raw = str(indicator or "").strip()
    upper = raw.upper()
    if "CPI" in upper:
        return "CPI"
    if "PPI" in upper:
        return "PPI"
    if "CORE PCE" in upper or "PCE" in upper:
        return "CORE_PCE"
    if "NFP" in upper or "非农" in raw:
        return "NFP"
    if "初请" in raw or "CLAIMS" in upper:
        return "INITIAL_CLAIMS"
    if "失业率" in raw or "UNEMPLOYMENT" in upper:
        return "UNEMPLOYMENT"
    if "利率" in raw or "RATE" in upper or "FOMC" in upper:
        return "FED_RATE"
    token = raw.split(" ", 1)[0].strip().upper()
    return token or "UNKNOWN"


def build_event_features_dataframe(
    macro_csv_path: str,
    lookback_days: int = 60,
    strict_event_time: bool = False,
    auto_fetch_if_empty: bool = False,
    include_empty_context: bool = False,
) -> pd.DataFrame:
    macro_df = pd.read_csv(macro_csv_path)
    if macro_df.empty:
        return pd.DataFrame()

    manager = BTCKlineManager()
    rows: List[Dict[str, Any]] = []
    feature_names: Optional[List[str]] = None

    for idx, macro_row in macro_df.iterrows():
        release_col = macro_row.get("release_time(utc+8)")
        event_ts = _to_beijing_datetime(release_col)
        if event_ts is None:
            continue

        sample = build_event_feature_sample(
            event_time=event_ts.to_pydatetime(),
            lookback_days=lookback_days,
            strict_event_time=strict_event_time,
            auto_fetch_if_empty=auto_fetch_if_empty,
            manager=manager,
        )

        current_names = sample.get("feature_names", [])
        vector = sample.get("feature_vector", [])
        if not isinstance(current_names, list) or not isinstance(vector, list):
            continue
        if len(current_names) != len(vector):
            continue
        if feature_names is None:
            feature_names = current_names

        tf_sizes = {}
        for tf in ("15min", "1h", "4h", "1d"):
            tf_sizes[f"{tf}_sample_size"] = int(
                sample.get("features_by_timeframe", {}).get(tf, {}).get("sample_size", 0)
            )
        has_kline_context = int(tf_sizes["15min_sample_size"] > 0)
        if not include_empty_context and not has_kline_context:
            continue

        record: Dict[str, Any] = {
            "row_index": int(idx),
            "event_time": sample.get("event_time"),
            "release_time(utc+8)": str(release_col),
            "indicator_key": macro_row.get("indicator"),
            "actual": macro_row.get("actual"),
            "forecast": macro_row.get("forecast"),
            "previous": macro_row.get("previous"),
            "updated_at": macro_row.get("updated_at"),
            "lookback_days": int(lookback_days),
            "has_kline_context": has_kline_context,
            "feature_vector": json.dumps(vector, ensure_ascii=False, separators=(",", ":")),
        }
        record.update(tf_sizes)

        for name, value in zip(current_names, vector):
            record[name] = float(value)

        rows.append(record)

    if not rows:
        return pd.DataFrame()

    result = pd.DataFrame(rows)
    order: List[str] = [
        "row_index",
        "event_time",
        "release_time(utc+8)",
        "indicator",
        "indicator_key",
        "actual",
        "forecast",
        "previous",
        "updated_at",
        "lookback_days",
        "has_kline_context",
        "15min_sample_size",
        "1h_sample_size",
        "4h_sample_size",
        "1d_sample_size",
        "feature_vector",
    ]
    if feature_names:
        order.extend(feature_names)
    cols = [c for c in order if c in result.columns]
    cols.extend([c for c in result.columns if c not in cols])
    return result[cols]


def main() -> None:
    parser = argparse.ArgumentParser(description="Build historical event features for BTC macro analysis.")
    parser.add_argument("--macro-csv", default=DEFAULT_MACRO_CSV, help="Path to macro_data.csv")
    parser.add_argument("--output-csv", default=DEFAULT_OUTPUT_CSV, help="Output path for event_features.csv")
    parser.add_argument("--lookback-days", type=int, default=60, help="Feature lookback window in days")
    parser.add_argument(
        "--strict-event-time",
        action="store_true",
        help="Compatibility flag kept for the same API semantics.",
    )
    parser.add_argument(
        "--auto-fetch-if-empty",
        action="store_true",
        help="Allow fetching missing K lines from API when local cache is empty.",
    )
    parser.add_argument(
        "--include-empty-context",
        action="store_true",
        help="Keep events that have no local K-line context (feature vector may be all zeros).",
    )
    args = parser.parse_args()

    output_dir = os.path.dirname(args.output_csv)
    os.makedirs(output_dir, exist_ok=True)

    df = build_event_features_dataframe(
        macro_csv_path=args.macro_csv,
        lookback_days=args.lookback_days,
        strict_event_time=args.strict_event_time,
        auto_fetch_if_empty=args.auto_fetch_if_empty,
        include_empty_context=args.include_empty_context,
    )
    df.to_csv(args.output_csv, index=False)
    print(f"[done] rows={len(df)} saved={args.output_csv}")


if __name__ == "__main__":
    main()
