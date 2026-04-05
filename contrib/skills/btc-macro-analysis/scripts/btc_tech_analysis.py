#!/usr/bin/env python3
"""
BTC 多周期技术特征构建

实现内容：
1. 按事件时刻（20:30 / 21:30）合成当前1h、4h、日线“已走部分”K线
2. 统一提取四个周期（15min/1h/4h/1d）的技术特征
3. 拼接输出大特征向量，支持保存为历史样本
"""

import json
import os
import sys
from datetime import datetime, timedelta, timezone
from typing import Any, Dict, List, Optional

import numpy as np
import pandas as pd

# 确保可以直接导入同目录脚本
SCRIPT_DIR = os.path.dirname(__file__)
if SCRIPT_DIR not in sys.path:
    sys.path.append(SCRIPT_DIR)

from btc_kline_manager import BTCKlineManager

BEIJING_TZ = timezone(timedelta(hours=8))
FEATURE_ORDER = [
    "close_ma20_ratio",
    "close_ma50_ratio",
    "close_ma99_ratio",
    "close_ma200_ratio",
    "bollinger_position",
    "ret_5",
    "ret_10",
    "ma20_slope",
    "ma50_slope",
    "adx14",
    "volatility_10",
    "rsi14",
    "macd_histogram",
    "short_trend_label",
    "long_trend_label",
]

STRUCTURE_SWING_WINDOW = 3
STRUCTURE_SWING_ATR_MULT = 1.0
STRUCTURE_RANGE_ATR_MULT = 2.0
STRUCTURE_SLOPE_ATR_MULT = 0.12

SHORT_LOOKBACK_MAP = {
    "15min": 48,
    "1h": 48,
    "4h": 24,
    "1d": 7,
}

LONG_LOOKBACK_MAP = {
    "15min": 96,
    "1h": 72,
    "4h": 36,
    "1d": 14,
}


def _ensure_beijing_datetime(dt: datetime) -> datetime:
    if dt.tzinfo is None:
        return dt.replace(tzinfo=BEIJING_TZ)
    return dt.astimezone(BEIJING_TZ)


def _as_dataframe(data: Any) -> pd.DataFrame:
    """将输入标准化为K线DataFrame"""
    columns = ["timestamp", "datetime", "open", "high", "low", "close", "volume", "quote_volume", "timeframe"]
    if data is None:
        return pd.DataFrame(columns=columns)

    if isinstance(data, pd.DataFrame):
        df = data.copy()
    else:
        df = pd.DataFrame(data)

    if df.empty:
        return pd.DataFrame(columns=columns)

    if "quote_volume" not in df.columns:
        df["quote_volume"] = 0.0
    if "timeframe" not in df.columns:
        df["timeframe"] = ""

    if "timestamp" in df.columns:
        df["timestamp"] = pd.to_numeric(df["timestamp"], errors="coerce")
    else:
        df["timestamp"] = np.nan

    if df["timestamp"].isna().all() and "datetime" in df.columns:
        dt_series = pd.to_datetime(df["datetime"], errors="coerce")
        if not dt_series.empty and dt_series.dt.tz is None:
            dt_series = dt_series.dt.tz_localize(BEIJING_TZ)
        elif not dt_series.empty:
            dt_series = dt_series.dt.tz_convert(BEIJING_TZ)
        df["timestamp"] = (dt_series.astype("int64") // 10**6).astype("float64")

    df["timestamp"] = df["timestamp"].fillna(0).astype("int64")
    df["datetime"] = pd.to_datetime(df["timestamp"], unit="ms", utc=True).dt.tz_convert(BEIJING_TZ)

    for col in ["open", "high", "low", "close", "volume", "quote_volume"]:
        if col not in df.columns:
            df[col] = 0.0
        df[col] = pd.to_numeric(df[col], errors="coerce").fillna(0.0)

    df = df.dropna(subset=["datetime"])
    df = df.sort_values("timestamp").drop_duplicates(subset=["timestamp"], keep="last").reset_index(drop=True)
    return df[columns]


def _safe_ratio(numerator: float, denominator: float, default: float = 1.0) -> float:
    if denominator == 0 or np.isnan(denominator):
        return float(default)
    return float(numerator / denominator)


def _calculate_rsi(closes: pd.Series, period: int = 14) -> float:
    if len(closes) < period + 1:
        return 50.0

    delta = closes.diff()
    gain = delta.clip(lower=0)
    loss = -delta.clip(upper=0)

    avg_gain = gain.ewm(alpha=1 / period, adjust=False, min_periods=period).mean()
    avg_loss = loss.ewm(alpha=1 / period, adjust=False, min_periods=period).mean()
    rs = avg_gain / avg_loss.replace(0, np.nan)
    rsi = 100 - (100 / (1 + rs))

    value = rsi.iloc[-1]
    if pd.isna(value):
        return 50.0
    return float(value)


def _calculate_macd(closes: pd.Series, fast: int = 12, slow: int = 26, signal: int = 9) -> Dict[str, float]:
    if closes.empty:
        return {
            "line": 0.0,
            "signal": 0.0,
            "histogram": 0.0,
            "golden_cross": 0.0,
            "death_cross": 0.0,
        }

    ema_fast = closes.ewm(span=fast, adjust=False).mean()
    ema_slow = closes.ewm(span=slow, adjust=False).mean()
    macd_line = ema_fast - ema_slow
    signal_line = macd_line.ewm(span=signal, adjust=False).mean()
    histogram = macd_line - signal_line

    if len(macd_line) < 2 or len(signal_line) < 2:
        golden_cross = 0.0
        death_cross = 0.0
    else:
        prev_macd = macd_line.iloc[-2]
        prev_signal = signal_line.iloc[-2]
        curr_macd = macd_line.iloc[-1]
        curr_signal = signal_line.iloc[-1]
        golden_cross = float(prev_macd <= prev_signal and curr_macd > curr_signal)
        death_cross = float(prev_macd >= prev_signal and curr_macd < curr_signal)

    return {
        "line": float(macd_line.iloc[-1]),
        "signal": float(signal_line.iloc[-1]),
        "histogram": float(histogram.iloc[-1]),
        "golden_cross": golden_cross,
        "death_cross": death_cross,
    }


def _calculate_atr_series(df: pd.DataFrame, period: int = 14) -> List[Optional[float]]:
    if df.empty:
        return []

    high = df["high"].astype(float)
    low = df["low"].astype(float)
    close = df["close"].astype(float)
    prev_close = close.shift(1)

    tr = pd.concat(
        [
            (high - low).abs(),
            (high - prev_close).abs(),
            (low - prev_close).abs(),
        ],
        axis=1,
    ).max(axis=1)
    atr = tr.ewm(alpha=1 / period, adjust=False, min_periods=period).mean()
    return atr.tolist()


def _calculate_swing_points(
    data: List[Dict[str, float]],
    atr: List[Optional[float]],
    window: int = STRUCTURE_SWING_WINDOW,
    atr_mult: float = STRUCTURE_SWING_ATR_MULT,
) -> List[Dict[str, Any]]:
    if len(data) < window * 2 + 1:
        return []

    swings: List[Dict[str, Any]] = []
    last_swing: Optional[Dict[str, Any]] = None

    for i in range(window, len(data) - window):
        high = data[i]["high"]
        low = data[i]["low"]

        prev_highs = [data[j]["high"] for j in range(i - window, i)]
        next_highs = [data[j]["high"] for j in range(i + 1, i + window + 1)]
        prev_lows = [data[j]["low"] for j in range(i - window, i)]
        next_lows = [data[j]["low"] for j in range(i + 1, i + window + 1)]

        is_swing_high = high >= max(prev_highs) and high >= max(next_highs)
        is_swing_low = low <= min(prev_lows) and low <= min(next_lows)
        if (not is_swing_high and not is_swing_low) or (is_swing_high and is_swing_low):
            continue

        candidate = {
            "type": "SH" if is_swing_high else "SL",
            "price": high if is_swing_high else low,
            "idx": i,
            "time": data[i].get("time"),
        }

        if last_swing is None:
            swings.append(candidate)
            last_swing = candidate
            continue

        if candidate["type"] == last_swing["type"]:
            if candidate["type"] == "SH" and candidate["price"] > last_swing["price"]:
                swings[-1] = candidate
                last_swing = candidate
            elif candidate["type"] == "SL" and candidate["price"] < last_swing["price"]:
                swings[-1] = candidate
                last_swing = candidate
            continue

        atr_value = atr[i] if atr and i < len(atr) else None
        if atr_value is None or (isinstance(atr_value, float) and np.isnan(atr_value)) or atr_value == 0:
            swings.append(candidate)
            last_swing = candidate
            continue

        if abs(candidate["price"] - last_swing["price"]) >= float(atr_value) * atr_mult:
            swings.append(candidate)
            last_swing = candidate

    return swings


def _calculate_structure_label(df: pd.DataFrame, timeframe: str, lookback: int) -> int:
    if len(df) < 10:
        return 0

    data = []
    for _, row in df.iterrows():
        data.append(
            {
                "high": float(row["high"]),
                "low": float(row["low"]),
                "close": float(row["close"]),
                "time": row.get("datetime"),
            }
        )

    atr = _calculate_atr_series(df, period=14)
    swings = _calculate_swing_points(data, atr)

    prev_high = None
    last_high = None
    prev_low = None
    last_low = None
    for sp in swings:
        if sp["type"] == "SH":
            prev_high = last_high
            last_high = sp["price"]
        else:
            prev_low = last_low
            last_low = sp["price"]

    market_state = "RANGE"
    if (
        prev_high is not None
        and prev_low is not None
        and last_high is not None
        and last_low is not None
    ):
        if last_high > prev_high and last_low > prev_low:
            market_state = "UP"
        elif last_high < prev_high and last_low < prev_low:
            market_state = "DOWN"

    lookback = min(max(lookback, 5), len(data) - 1)
    if lookback >= 5:
        window_data = data[-lookback:]
        highs = [c["high"] for c in window_data]
        lows = [c["low"] for c in window_data]
        window_range = max(highs) - min(lows)

        atr_window = [a for a in atr[-lookback:] if a is not None and not (isinstance(a, float) and np.isnan(a))]
        atr_mean = (sum(atr_window) / len(atr_window)) if atr_window else None

        closes = np.array([c["close"] for c in window_data], dtype=float)
        x = np.arange(len(closes))
        slope = np.polyfit(x, closes, 1)[0] if len(closes) > 1 else 0.0

        if atr_mean and atr_mean > 0:
            range_ok = window_range > STRUCTURE_RANGE_ATR_MULT * atr_mean
            slope_ok = abs(slope) > STRUCTURE_SLOPE_ATR_MULT * atr_mean
            if not (range_ok and slope_ok):
                market_state = "RANGE"
            else:
                slope_state = "UP" if slope > 0 else "DOWN"
                if market_state == "RANGE" or market_state != slope_state:
                    market_state = slope_state

    if market_state == "UP":
        return 1
    if market_state == "DOWN":
        return -1
    return 0


def _calculate_ma_slope(closes: pd.Series, ma_period: int, slope_window: int = 5) -> float:
    ma = closes.rolling(ma_period).mean().dropna()
    if len(ma) < 2:
        return 0.0
    window = min(slope_window, len(ma))
    y = ma.tail(window).to_numpy(dtype=float)
    x = np.arange(len(y), dtype=float)
    slope = np.polyfit(x, y, 1)[0] if len(y) > 1 else 0.0
    base = float(ma.iloc[-1]) if float(ma.iloc[-1]) != 0 else 1.0
    return float(slope / base)


def _calculate_adx14(df: pd.DataFrame, period: int = 14) -> float:
    if len(df) < period + 1:
        return 0.0

    high = df["high"].astype(float)
    low = df["low"].astype(float)
    close = df["close"].astype(float)

    up_move = high.diff()
    down_move = -low.diff()

    plus_dm = pd.Series(
        np.where((up_move > down_move) & (up_move > 0), up_move, 0.0),
        index=df.index,
    )
    minus_dm = pd.Series(
        np.where((down_move > up_move) & (down_move > 0), down_move, 0.0),
        index=df.index,
    )

    tr = pd.concat(
        [
            (high - low).abs(),
            (high - close.shift(1)).abs(),
            (low - close.shift(1)).abs(),
        ],
        axis=1,
    ).max(axis=1)

    atr = tr.ewm(alpha=1 / period, adjust=False, min_periods=period).mean()
    plus_di = 100 * plus_dm.ewm(alpha=1 / period, adjust=False, min_periods=period).mean() / atr.replace(0, np.nan)
    minus_di = 100 * minus_dm.ewm(alpha=1 / period, adjust=False, min_periods=period).mean() / atr.replace(0, np.nan)
    dx = (100 * (plus_di - minus_di).abs() / (plus_di + minus_di).replace(0, np.nan)).replace([np.inf, -np.inf], np.nan)
    adx = dx.ewm(alpha=1 / period, adjust=False, min_periods=period).mean()
    value = adx.iloc[-1] if not adx.empty else 0.0
    return 0.0 if pd.isna(value) else float(value)


def _build_feature_dict(df: pd.DataFrame, timeframe: str, volatility_window: int = 10) -> Dict[str, float]:
    if df.empty:
        return {name: 0.0 for name in FEATURE_ORDER}

    closes = df["close"]
    latest_close = float(closes.iloc[-1])

    ma20 = float(closes.rolling(20).mean().iloc[-1]) if len(closes) >= 20 else latest_close
    ma50 = float(closes.rolling(50).mean().iloc[-1]) if len(closes) >= 50 else latest_close
    ma99 = float(closes.rolling(99).mean().iloc[-1]) if len(closes) >= 99 else latest_close
    ma200 = float(closes.rolling(200).mean().iloc[-1]) if len(closes) >= 200 else latest_close

    bb_mid = float(closes.rolling(20).mean().iloc[-1]) if len(closes) >= 20 else latest_close
    bb_std = float(closes.rolling(20).std(ddof=0).iloc[-1]) if len(closes) >= 20 else 0.0
    bb_upper = bb_mid + 2.0 * bb_std
    bb_lower = bb_mid - 2.0 * bb_std

    close_5 = float(closes.iloc[-6]) if len(closes) >= 6 else latest_close
    close_10 = float(closes.iloc[-11]) if len(closes) >= 11 else latest_close

    rsi_value = _calculate_rsi(closes, 14)
    macd = _calculate_macd(closes)
    adx14 = _calculate_adx14(df, period=14)

    returns = closes.pct_change().dropna()
    volatility_10 = float(returns.tail(volatility_window).std(ddof=0)) if not returns.empty else 0.0

    if bb_upper > bb_lower:
        bollinger_position = float((latest_close - bb_lower) / (bb_upper - bb_lower))
    else:
        bollinger_position = 0.5
    bollinger_position = float(np.clip(bollinger_position, 0.0, 1.0))

    short_lookback = SHORT_LOOKBACK_MAP.get(timeframe, 48)
    long_lookback = LONG_LOOKBACK_MAP.get(timeframe, 96)
    short_trend_label = float(_calculate_structure_label(df, timeframe, short_lookback))
    long_trend_label = float(_calculate_structure_label(df, timeframe, long_lookback))

    feature_map = {
        "close_ma20_ratio": _safe_ratio(latest_close, ma20),
        "close_ma50_ratio": _safe_ratio(latest_close, ma50),
        "close_ma99_ratio": _safe_ratio(latest_close, ma99),
        "close_ma200_ratio": _safe_ratio(latest_close, ma200),
        "bollinger_position": bollinger_position,
        "ret_5": _safe_ratio(latest_close, close_5) - 1.0,
        "ret_10": _safe_ratio(latest_close, close_10) - 1.0,
        "ma20_slope": _calculate_ma_slope(closes, ma_period=20, slope_window=5),
        "ma50_slope": _calculate_ma_slope(closes, ma_period=50, slope_window=5),
        "adx14": adx14,
        "volatility_10": float(volatility_10),
        "rsi14": float(rsi_value),
        "macd_histogram": float(macd["histogram"]),
        "short_trend_label": short_trend_label,
        "long_trend_label": long_trend_label,
    }
    return feature_map


def extract_timeframe_features(dataframe: pd.DataFrame, timeframe: str) -> Dict[str, Any]:
    """
    提取单周期特征，输出固定顺序向量。
    """
    df = _as_dataframe(dataframe)
    features = _build_feature_dict(df, timeframe=timeframe)
    vector = np.array([features[name] for name in FEATURE_ORDER], dtype=float)
    feature_names = [f"{timeframe}_{name}" for name in FEATURE_ORDER]
    return {
        "timeframe": timeframe,
        "feature_names": feature_names,
        "feature_map": features,
        "feature_vector": vector,
        "sample_size": int(len(df)),
        "latest_timestamp": int(df["timestamp"].iloc[-1]) if not df.empty else None,
    }


def build_event_feature_sample(
    event_time: datetime,
    lookback_days: int = 60,
    auto_fetch_if_empty: bool = True,
    manager: Optional[BTCKlineManager] = None,
) -> Dict[str, Any]:
    """
    生成事件时刻的四周期拼接特征向量。
    """
    event_time = _ensure_beijing_datetime(event_time)

    manager = manager or BTCKlineManager()
    event_klines = manager.build_event_klines(
        event_time=event_time,
        lookback_days=lookback_days,
        auto_fetch_if_empty=auto_fetch_if_empty,
    )

    timeframes = ["15min", "1h", "4h", "1d"]
    per_tf: Dict[str, Any] = {}
    vectors: List[np.ndarray] = []
    names: List[str] = []

    for tf in timeframes:
        tf_df = _as_dataframe(event_klines.get(tf, []))
        tf_features = extract_timeframe_features(tf_df, tf)
        per_tf[tf] = {
            "feature_names": tf_features["feature_names"],
            "feature_map": tf_features["feature_map"],
            "feature_vector": tf_features["feature_vector"].tolist(),
            "sample_size": tf_features["sample_size"],
            "latest_timestamp": tf_features["latest_timestamp"],
        }
        vectors.append(tf_features["feature_vector"])
        names.extend(tf_features["feature_names"])

    if vectors:
        feature_vector = np.concatenate(vectors)
    else:
        feature_vector = np.array([], dtype=float)

    return {
        "event_time": event_klines.get("event_time", event_time.isoformat()),
        "lookback_days": lookback_days,
        "klines": event_klines,
        "feature_names": names,
        "feature_vector": feature_vector.tolist(),
        "features_by_timeframe": per_tf,
    }


def save_sample(
    event_time: datetime,
    feature_vector: List[float],
    feature_names: Optional[List[str]] = None,
    output_dir: Optional[str] = None,
    extra: Optional[Dict[str, Any]] = None
) -> str:
    """将样本保存为JSON，供后续相似度匹配使用"""
    event_time = _ensure_beijing_datetime(event_time)
    output_dir = output_dir or os.path.join(SCRIPT_DIR, "..", "data", "btc_feature_samples")
    os.makedirs(output_dir, exist_ok=True)

    filename = f"sample_{event_time.strftime('%Y%m%dT%H%M%S_BJ')}.json"
    path = os.path.join(output_dir, filename)

    payload: Dict[str, Any] = {
        "event_time": event_time.isoformat(),
        "saved_at": datetime.now(BEIJING_TZ).isoformat(),
        "feature_vector": [float(x) for x in feature_vector],
    }
    if feature_names is not None:
        payload["feature_names"] = feature_names
    if extra:
        payload["extra"] = extra

    with open(path, "w", encoding="utf-8") as f:
        json.dump(payload, f, ensure_ascii=False, separators=(",", ":"))
    return path


def _extract_indicator_key(sample_payload: Dict[str, Any]) -> Optional[str]:
    """
    兼容两种存储结构：
    1) 顶层 indicator_key
    2) extra.indicator_key
    """
    key = sample_payload.get("indicator_key")
    if key:
        return str(key)
    extra = sample_payload.get("extra")
    if isinstance(extra, dict) and extra.get("indicator_key"):
        return str(extra.get("indicator_key"))
    return None


def cosine_similarity(vec_a: List[float], vec_b: List[float]) -> float:
    """
    计算余弦相似度，返回范围 [-1, 1]。
    """
    a = np.array(vec_a, dtype=float)
    b = np.array(vec_b, dtype=float)
    if a.size == 0 or b.size == 0 or a.size != b.size:
        return 0.0
    if np.isnan(a).any() or np.isnan(b).any():
        return 0.0
    denom = np.linalg.norm(a) * np.linalg.norm(b)
    if denom == 0:
        return 0.0
    return float(np.dot(a, b) / denom)


def load_event_features(
    sample_dir: Optional[str] = None,
    indicator_key: Optional[str] = None,
) -> List[Dict[str, Any]]:
    """
    读取本地历史特征样本（仅支持 CSV），可按 indicator_key 过滤。
    """
    default_csv_file = os.path.join(SCRIPT_DIR, "..", "data", "event_features.csv")

    def _normalize_feature_vector(value: Any) -> List[float]:
        vec: List[Any]
        if isinstance(value, list):
            vec = value
        elif isinstance(value, str):
            text = value.strip()
            if not text:
                return []
            try:
                parsed = json.loads(text)
            except Exception:
                return []
            if not isinstance(parsed, list):
                return []
            vec = parsed
        else:
            return []

        normalized: List[float] = []
        for item in vec:
            try:
                normalized.append(float(item))
            except Exception:
                return []
        return normalized

    def _load_csv_file(csv_path: str) -> List[Dict[str, Any]]:
        items: List[Dict[str, Any]] = []
        try:
            df = pd.read_csv(csv_path)
        except Exception:
            return items

        for idx, row in df.iterrows():
            vector = _normalize_feature_vector(row.get("feature_vector", ""))
            if len(vector) == 0:
                continue

            row_indicator_key = row.get("indicator_key")
            sample_indicator = str(row_indicator_key).strip() if pd.notna(row_indicator_key) else None
            if not sample_indicator:
                sample_indicator = None
            if indicator_key and sample_indicator != indicator_key:
                continue

            items.append(
                {
                    "event_time": row.get("event_time"),
                    "indicator": row.get("indicator"),
                    "indicator_key": sample_indicator,
                    "feature_vector": vector,
                    "data": {
                        "actual": row.get("actual"),
                        "forecast": row.get("forecast"),
                        "previous": row.get("previous"),
                        "sample_file": f"{os.path.basename(csv_path)}#{idx}",
                    },
                }
            )
        return items

    if sample_dir:
        if os.path.isdir(sample_dir):
            csv_path = os.path.join(sample_dir, "event_features.csv")
        else:
            csv_path = sample_dir
    else:
        csv_path = default_csv_file

    if not os.path.isfile(csv_path):
        return []

    return _load_csv_file(csv_path)


def find_similar_feature_samples(
    current_sample: Dict[str, Any],
    top_k: int = 3,
    sample_dir: Optional[str] = None,
    indicator_key: Optional[str] = None,
    prefer_same_indicator: bool = True,
) -> List[Dict[str, Any]]:
    """
    基于当前样本向量在历史样本库中做余弦相似度匹配。
    """
    current_vector = current_sample.get("feature_vector", [])
    if not isinstance(current_vector, list) or len(current_vector) == 0:
        return []

    all_samples = load_event_features(sample_dir=sample_dir)
    if not all_samples:
        return []

    same_indicator_results: List[Dict[str, Any]] = []
    cross_indicator_results: List[Dict[str, Any]] = []

    for sample in all_samples:
        hist_vector = sample.get("feature_vector", [])
        if not isinstance(hist_vector, list) or len(hist_vector) != len(current_vector):
            continue
        sim = cosine_similarity(current_vector, hist_vector)
        item = {
            "event_time": sample.get("event_time"),
            "indicator_key": sample.get("indicator_key"),
            "indicator": sample.get("indicator"),
            "similarity": sim,
            "data": sample.get("data", {}),
        }
        if indicator_key and sample.get("indicator_key") == indicator_key:
            same_indicator_results.append(item)
        else:
            cross_indicator_results.append(item)

    if indicator_key and prefer_same_indicator and same_indicator_results:
        ranking = same_indicator_results
    else:
        ranking = same_indicator_results + cross_indicator_results

    ranking.sort(key=lambda x: x["similarity"], reverse=True)
    return ranking[: max(1, int(top_k))]


def match_similar_events(
    event_time: datetime,
    lookback_days: int = 60,
    auto_fetch_if_empty: bool = True,
    top_k: int = 3,
    sample_dir: Optional[str] = None,
    indicator_key: Optional[str] = None,
    prefer_same_indicator: bool = True,
) -> Dict[str, Any]:
    """
    一步完成：生成当前 pre 特征 + 历史样本相似度匹配。
    """
    current_sample = build_event_feature_sample(
        event_time=event_time,
        lookback_days=lookback_days,
        auto_fetch_if_empty=auto_fetch_if_empty,
    )
    matches = find_similar_feature_samples(
        current_sample=current_sample,
        top_k=top_k,
        sample_dir=sample_dir,
        indicator_key=indicator_key,
        prefer_same_indicator=prefer_same_indicator,
    )
    return {
        "event_time": current_sample.get("event_time"),
        "indicator_key": indicator_key,
        "top_k": top_k,
        "matches": matches,
        "current_sample": current_sample,
    }


def build_technical_snapshot(
    event_time: datetime,
    lookback_days: int = 60,
    auto_fetch_if_empty: bool = True,
) -> Dict[str, Any]:
    """
    输出简化版技术快照（四周期核心字段），便于报告展示。
    """
    sample = build_event_feature_sample(
        event_time=event_time,
        lookback_days=lookback_days,
        auto_fetch_if_empty=auto_fetch_if_empty,
    )

    snapshot: Dict[str, Dict[str, float]] = {}
    for tf in ["15min", "1h", "4h", "1d"]:
        feature_map = sample.get("features_by_timeframe", {}).get(tf, {}).get("feature_map", {})
        snapshot[tf] = {
            "close_ma20_ratio": float(feature_map.get("close_ma20_ratio", 0.0)),
            "rsi14": float(feature_map.get("rsi14", 50.0)),
            "volatility_10": float(feature_map.get("volatility_10", 0.0)),
            "short_trend_label": float(feature_map.get("short_trend_label", 0.0)),
            "long_trend_label": float(feature_map.get("long_trend_label", 0.0)),
        }

    return {
        "event_time": sample.get("event_time"),
        "technical_snapshot": snapshot,
    }


def get_features_15m(dataframe: pd.DataFrame) -> np.ndarray:
    return extract_timeframe_features(dataframe, "15min")["feature_vector"]


def get_features_1h(dataframe: pd.DataFrame) -> np.ndarray:
    return extract_timeframe_features(dataframe, "1h")["feature_vector"]


def get_features_4h(dataframe: pd.DataFrame) -> np.ndarray:
    return extract_timeframe_features(dataframe, "4h")["feature_vector"]


def get_features_day(dataframe: pd.DataFrame) -> np.ndarray:
    return extract_timeframe_features(dataframe, "1d")["feature_vector"]


def get_tech_analysis(dataframe: pd.DataFrame):
    """获取指定DataFrame的技术特征"""
    return extract_timeframe_features(dataframe, "custom")


def get_pre_dt_tech_analysis(request_dt: datetime):
    """
    获取指定时间点（含该时刻已闭合K线）的多周期技术分析。
    """
    return build_event_feature_sample(request_dt, lookback_days=60)


def get_post_dt_tech_analysis(request_dt: datetime):
    """
    获取指定时间之后15分钟的多周期技术分析。
    可用于事件后短期对比。
    """
    return build_event_feature_sample(
        request_dt + timedelta(minutes=15),
        lookback_days=60,
    )


def get_technical_analysis(dt: datetime):
    """
    对外统一入口：生成四周期特征向量及对应K线样本。
    """
    return build_event_feature_sample(dt, lookback_days=60)