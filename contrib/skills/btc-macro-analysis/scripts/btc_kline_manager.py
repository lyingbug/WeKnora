#!/usr/bin/env python3
"""
BTC K线数据维护系统
功能：
1. 多时间框架历史数据获取与维护 (15m/1h/4h/1d)
2. 增量更新，避免重复下载
3. 数据完整性检查与修复
4. 高效查询接口
"""

import os
import pandas as pd
import requests
from datetime import datetime, timezone, timedelta
from typing import Dict, List, Optional, Tuple, Any
import time

# 配置
DATA_DIR = os.path.join(os.path.dirname(__file__), '..', 'data', 'btc_klines')
BITGET_API_BASE = "https://api.bitget.com/api/v2/mix/market"
CONTRACT_SYMBOL = "BTCUSDT"
CONTRACT_PRODUCT_TYPE = "USDT-FUTURES"
CST = timezone(timedelta(hours=8))

# 支持的时间框架
TIMEFRAMES = {
    "15min": {"granularity": "15m",   "limit": 200, "retention_days": 365},
    "1h":    {"granularity": "1H",    "limit": 200, "retention_days": 365},
    "4h":    {"granularity": "4H",    "limit": 200, "retention_days": 365},
    "1d":    {"granularity": "1D",    "limit": 200, "retention_days": 365},
}

TIMEFRAME_MINUTES = {
    "15min": 15,
    "1h": 60,
    "4h": 240,
    "1d": 1440,
}

# 初始化历史下载起点（UTC）
INIT_HISTORY_START_UTC = datetime(2021, 1, 1, tzinfo=timezone.utc)
# 缺口检测默认回看窗口
MIN_COVERAGE_DAYS = 365
MAX_GAP_FILL_RANGES_PER_PASS = 200
MAX_GAP_FILL_PASSES = 3

class BTCKlineManager:
    def __init__(self):
        self.ensure_directories()

    def ensure_directories(self):
        """确保数据目录存在"""
        for tf in TIMEFRAMES.keys():
            tf_dir = os.path.join(DATA_DIR, tf)
            os.makedirs(tf_dir, exist_ok=True)

    def get_data_file_path(self, timeframe: str) -> str:
        """获取单周期CSV文件路径（每个周期一个文件）"""
        return os.path.join(DATA_DIR, timeframe, f"btc_{timeframe}.csv")

    def _to_ts_ms(self, dt: datetime) -> int:
        return int(self._ensure_beijing_datetime(dt).timestamp() * 1000)

    def _from_ts_ms(self, ts_ms: int) -> datetime:
        return datetime.fromtimestamp(int(ts_ms) / 1000, tz=CST)

    def _timeframe_step_ms(self, timeframe: str) -> int:
        return TIMEFRAME_MINUTES[timeframe] * 60 * 1000

    def _write_timeframe_dataframe(self, timeframe: str, df: pd.DataFrame) -> None:
        """写入单周期CSV文件"""
        file_path = self.get_data_file_path(timeframe)
        save_df = df.copy()
        if 'timestamp' in save_df.columns:
            save_df['datetime'] = pd.to_datetime(save_df['timestamp'], unit='ms', utc=True).dt.tz_convert(CST).dt.strftime('%Y-%m-%dT%H:%M:%S%z')
        save_df.to_csv(file_path, index=False)

    def _load_timeframe_dataframe(self, timeframe: str) -> pd.DataFrame:
        """
        读取单周期CSV；若不存在则尝试从旧JSON文件迁移。
        """
        file_path = self.get_data_file_path(timeframe)
        if os.path.exists(file_path):
            try:
                df = pd.read_csv(file_path)
                return self._normalize_candle_dataframe(df.to_dict('records'))
            except Exception:
                pass
            return self._normalize_candle_dataframe([])
        else:
            return pd.DataFrame()

    def _build_bitget_history_params(
        self,
        timeframe: str,
        limit: int,
        start_ts: Optional[int] = None,
        end_ts: Optional[int] = None
    ) -> Dict[str, Any]:
        config = TIMEFRAMES[timeframe]
        params: Dict[str, Any] = {
            'symbol': CONTRACT_SYMBOL,
            'productType': CONTRACT_PRODUCT_TYPE,
            'granularity': config['granularity'],
            'limit': limit,
        }
        if start_ts:
            params['startTime'] = start_ts
        if end_ts:
            params['endTime'] = end_ts
        return params

    def _get_json(
        self,
        url: str,
        params: Optional[Dict[str, Any]] = None,
        timeout: int = 30
    ) -> Dict[str, Any]:
        """发起GET请求并返回JSON响应"""
        response = requests.get(url, params=params, timeout=timeout)
        response.raise_for_status()
        return response.json()

    def _convert_bitget_rows_to_candles(self, rows: List[List[Any]], timeframe: str) -> List[Dict[str, Any]]:
        candles: List[Dict[str, Any]] = []
        for row in reversed(rows):  # Bitget返回倒序，需要正序
            quote_volume = 0.0
            if len(row) > 6:
                try:
                    quote_volume = float(row[6])
                except (ValueError, TypeError):
                    quote_volume = 0.0
            ts_ms = int(row[0])
            candles.append(
                {
                    'timestamp': ts_ms,
                    'datetime': self._from_ts_ms(ts_ms).isoformat(),
                    'open': float(row[1]),
                    'high': float(row[2]),
                    'low': float(row[3]),
                    'close': float(row[4]),
                    'volume': float(row[5]),
                    'quote_volume': quote_volume,
                    'timeframe': timeframe
                }
            )
        return candles

    def call_bitget_api(self, timeframe: str, start_ts: int = None, end_ts: int = None, limit: int = 200) -> List[Dict]:
        """调用Bitget合约历史K线API获取数据"""
        url = f"{BITGET_API_BASE}/history-candles"
        params = self._build_bitget_history_params(
            timeframe=timeframe,
            limit=limit,
            start_ts=start_ts,
            end_ts=end_ts
        )

        try:
            data = self._get_json(url, params=params, timeout=30)

            if data.get('code') != '00000' or not data.get('data'):
                print(f"[API Error] {data}")
                return []
            return self._convert_bitget_rows_to_candles(data['data'], timeframe)

        except Exception as e:
            print(f"[API Error] 获取{timeframe}数据失败: {e}")
            return []

    def get_existing_data_range(self, timeframe: str) -> Tuple[Optional[int], Optional[int]]:
        """获取现有数据的时间范围"""
        df = self._load_timeframe_dataframe(timeframe)
        if df.empty:
            return None, None

        return int(df['timestamp'].min()), int(df['timestamp'].max())

    def save_candles(self, timeframe: str, candles: List[Dict]):
        """保存K线数据（单周期一个CSV）"""
        if not candles:
            return

        existing_df = self._load_timeframe_dataframe(timeframe)
        incoming_df = self._normalize_candle_dataframe(candles)
        if incoming_df.empty:
            return

        if not existing_df.empty:
            merged = pd.concat([existing_df, incoming_df], ignore_index=True)
        else:
            merged = incoming_df

        merged = merged.sort_values('timestamp').drop_duplicates(subset=['timestamp'], keep='last').reset_index(drop=True)
        self._write_timeframe_dataframe(timeframe, merged)

    def query_candles(self, timeframe: str, start_time: datetime = None, end_time: datetime = None, limit: int = None) -> List[Dict]:
        """查询K线数据"""
        if start_time is None:
            start_time = datetime.now(CST) - timedelta(days=30)
        if end_time is None:
            end_time = datetime.now(CST)

        start_ts = self._to_ts_ms(start_time)
        end_ts = self._to_ts_ms(end_time)
        df = self._load_timeframe_dataframe(timeframe)
        if df.empty:
            return []

        filtered = df[(df['timestamp'] >= start_ts) & (df['timestamp'] <= end_ts)].copy()
        filtered = filtered.sort_values('timestamp').reset_index(drop=True)
        all_candles = self._dataframe_to_candles(filtered, timeframe)
        if limit:
            all_candles = all_candles[-limit:]

        return all_candles

    def _ensure_beijing_datetime(self, dt: datetime) -> datetime:
        """标准化为北京时间"""
        if dt.tzinfo is None:
            return dt.replace(tzinfo=CST)
        return dt.astimezone(CST)

    def _normalize_candle_dataframe(self, candles: List[Dict]) -> pd.DataFrame:
        """将K线列表标准化为DataFrame"""
        columns = ['timestamp', 'datetime', 'open', 'high', 'low', 'close', 'volume', 'quote_volume', 'timeframe']
        if not candles:
            return pd.DataFrame(columns=columns)

        df = pd.DataFrame(candles).copy()

        if 'quote_volume' not in df.columns:
            df['quote_volume'] = 0.0
        if 'timeframe' not in df.columns:
            df['timeframe'] = ''

        for col in ['open', 'high', 'low', 'close', 'volume', 'quote_volume']:
            if col not in df.columns:
                df[col] = 0.0
            df[col] = pd.to_numeric(df[col], errors='coerce').fillna(0.0)

        if 'timestamp' in df.columns:
            df['timestamp'] = pd.to_numeric(df['timestamp'], errors='coerce').fillna(0).astype('int64')
            df['datetime'] = pd.to_datetime(df['timestamp'], unit='ms', utc=True).dt.tz_convert(CST)
        else:
            dt_series = pd.to_datetime(df.get('datetime'), errors='coerce')
            if dt_series.dt.tz is None:
                dt_series = dt_series.dt.tz_localize(CST)
            else:
                dt_series = dt_series.dt.tz_convert(CST)
            df['datetime'] = dt_series
            df['timestamp'] = (df['datetime'].astype('int64') // 10**6).astype('int64')

        df = df.dropna(subset=['datetime'])
        df = df.sort_values('timestamp').drop_duplicates(subset=['timestamp'], keep='last').reset_index(drop=True)
        return df[columns]

    def _dataframe_to_candles(self, df: pd.DataFrame, timeframe: str) -> List[Dict[str, Any]]:
        """将DataFrame转回可序列化K线结构"""
        if df.empty:
            return []

        candles: List[Dict[str, Any]] = []
        for _, row in df.sort_values('timestamp').iterrows():
            item: Dict[str, Any] = {
                'timestamp': int(row['timestamp']),
                'datetime': pd.Timestamp(row['datetime']).tz_convert(CST).isoformat(),
                'open': float(row['open']),
                'high': float(row['high']),
                'low': float(row['low']),
                'close': float(row['close']),
                'volume': float(row['volume']),
                'quote_volume': float(row.get('quote_volume', 0.0)),
                'timeframe': row.get('timeframe') or timeframe,
            }
            if 'is_partial' in df.columns:
                value = row.get('is_partial', False)
                if pd.notna(value) and bool(value):
                    item['is_partial'] = True
            candles.append(item)
        return candles

    def load_data(
        self,
        timeframe: str,
        start_time: datetime = None,
        end_time: datetime = None,
        limit: int = None,
        lookback_days: int = MIN_COVERAGE_DAYS,
        auto_fetch_if_empty: bool = True
    ) -> pd.DataFrame:
        """
        加载本地K线数据；若本地为空可自动回源API。
        """
        now = datetime.now(CST)
        start_time = self._ensure_beijing_datetime(start_time or (now - timedelta(days=lookback_days)))
        end_time = self._ensure_beijing_datetime(end_time or now)

        candles = self.query_candles(timeframe, start_time=start_time, end_time=end_time, limit=limit)
        if auto_fetch_if_empty:
            self.update_timeframe(timeframe, lookback_days=MIN_COVERAGE_DAYS, start_time=start_time, end_time=end_time)
            candles = self.query_candles(timeframe, start_time=start_time, end_time=end_time, limit=limit)

        df = self._normalize_candle_dataframe(candles)
        if limit and not df.empty:
            df = df.tail(limit).reset_index(drop=True)
        return df

    def _filter_closed_candles(self, df: pd.DataFrame, timeframe: str, timestamp: datetime) -> pd.DataFrame:
        """过滤出在指定时间点之前已闭合的K线"""
        if df.empty:
            return df.copy()
        close_delta = pd.to_timedelta(TIMEFRAME_MINUTES[timeframe], unit='m')
        ts = pd.Timestamp(self._ensure_beijing_datetime(timestamp))
        mask = (df['datetime'] + close_delta) <= ts
        return df.loc[mask].copy().reset_index(drop=True)

    def _aggregate_15m_window(self, df_15m: pd.DataFrame, window_start: datetime, window_end: datetime, timeframe: str) -> Optional[Dict[str, Any]]:
        """
        使用15分钟K线合成指定窗口内的“已走部分K线”。
        规则：只取 window_end 时刻前已闭合的15min。
        """
        if df_15m.empty:
            return None

        window_start_ts = pd.Timestamp(self._ensure_beijing_datetime(window_start))
        window_end_ts = pd.Timestamp(self._ensure_beijing_datetime(window_end))
        close_15m = pd.to_timedelta(15, unit='m')

        mask = (
            (df_15m['datetime'] >= window_start_ts) &
            ((df_15m['datetime'] + close_15m) <= window_end_ts)
        )
        window_df = df_15m.loc[mask].copy()
        if window_df.empty:
            return None

        window_df = window_df.sort_values('datetime').reset_index(drop=True)
        return {
            'timestamp': int(window_start_ts.timestamp() * 1000),
            'datetime': window_start_ts.isoformat(),
            'open': float(window_df.iloc[0]['open']),
            'high': float(window_df['high'].max()),
            'low': float(window_df['low'].min()),
            'close': float(window_df.iloc[-1]['close']),
            'volume': float(window_df['volume'].sum()),
            'quote_volume': float(window_df['quote_volume'].sum()),
            'timeframe': timeframe,
            'is_partial': True,
        }

    def synthesize_1h_from_15m(self, df_15m: pd.DataFrame, timestamp: datetime) -> Optional[Dict[str, Any]]:
        """合成当前1h（已走部分）K线"""
        ts = self._ensure_beijing_datetime(timestamp)
        hour_start = ts.replace(minute=0, second=0, microsecond=0)
        return self._aggregate_15m_window(df_15m, hour_start, ts, '1h')

    def synthesize_4h_from_15m(self, df_15m: pd.DataFrame, timestamp: datetime) -> Optional[Dict[str, Any]]:
        """合成当前4h（已走部分）K线，固定切片 00/04/08/12/16/20"""
        ts = self._ensure_beijing_datetime(timestamp)
        block_hour = (ts.hour // 4) * 4
        start_4h = ts.replace(hour=block_hour, minute=0, second=0, microsecond=0)
        return self._aggregate_15m_window(df_15m, start_4h, ts, '4h')

    def synthesize_day_from_15m(self, df_15m: pd.DataFrame, timestamp: datetime) -> Optional[Dict[str, Any]]:
        """合成当前日线（已走部分）K线"""
        ts = self._ensure_beijing_datetime(timestamp)
        day_start = ts.replace(hour=0, minute=0, second=0, microsecond=0)
        return self._aggregate_15m_window(df_15m, day_start, ts, '1d')

    def build_event_klines(
        self,
        event_time: datetime,
        lookback_days: int = 60,
        auto_fetch_if_empty: bool = True
    ) -> Dict[str, Any]:
        """
        构建事件时刻的四周期K线上下文（15m / 1h / 4h / 1d）。
        规则：
        - 15m：保留事件时刻前已闭合的原始15m
        - 1h/4h/1d：保留历史已闭合K线 + 当前周期已走部分（由15m合成）
        """
        event_time = self._ensure_beijing_datetime(event_time)
        start_time = event_time - timedelta(days=lookback_days)
        preload_start = start_time - timedelta(days=2)
        df_15m = self.load_data(
            '15min',
            start_time=preload_start,
            end_time=event_time + timedelta(minutes=15),
            auto_fetch_if_empty=auto_fetch_if_empty
        )
        df_15m_closed = self._filter_closed_candles(df_15m, '15min', event_time)
        df_15m_window = df_15m_closed[df_15m_closed['datetime'] >= pd.Timestamp(start_time)].copy()

        result = {
            'event_time': event_time.isoformat(),
            '15min': self._dataframe_to_candles(df_15m_window, '15min')
        }

        for tf in ['1h', '4h', '1d']:
            df_tf = self.load_data(
                tf,
                start_time=preload_start,
                end_time=event_time + timedelta(days=1),
                auto_fetch_if_empty=auto_fetch_if_empty
            )
            df_tf_closed = self._filter_closed_candles(df_tf, tf, event_time)
            df_tf_closed = df_tf_closed[df_tf_closed['datetime'] >= pd.Timestamp(start_time)].copy()

            if tf == '1h':
                partial = self.synthesize_1h_from_15m(df_15m_closed, event_time)
            elif tf == '4h':
                partial = self.synthesize_4h_from_15m(df_15m_closed, event_time)
            else:
                partial = self.synthesize_day_from_15m(df_15m_closed, event_time)

            if partial:
                df_partial = self._normalize_candle_dataframe([partial])
                df_partial['is_partial'] = True
                df_partial['timeframe'] = tf

                # 同时间戳优先保留合成的当前周期K线
                df_tf_closed = df_tf_closed[df_tf_closed['timestamp'] != partial['timestamp']]
                if df_tf_closed.empty:
                    df_tf_closed = df_partial.copy()
                else:
                    df_tf_closed = pd.concat([df_tf_closed, df_partial], ignore_index=True)
                df_tf_closed = df_tf_closed.sort_values('timestamp').reset_index(drop=True)

            result[tf] = self._dataframe_to_candles(df_tf_closed, tf)

        return result

    def build_event_candlesticks(
        self,
        event_time: datetime,
        lookback_days: int = 60,
        auto_fetch_if_empty: bool = True
    ) -> Dict[str, Any]:
        """
        兼容旧调用名，等价于 build_event_klines。
        """
        return self.build_event_klines(
            event_time=event_time,
            lookback_days=lookback_days,
            auto_fetch_if_empty=auto_fetch_if_empty
        )

    def _get_event_reference_price(self, event_time: datetime, auto_fetch_if_empty: bool = True) -> Optional[float]:
        """
        获取事件发生时刻的参考价：使用事件前最后一根已闭合15min的收盘价。
        """
        event_time = self._ensure_beijing_datetime(event_time)
        df_15m = self.load_data(
            '15min',
            start_time=event_time - timedelta(hours=6),
            end_time=event_time + timedelta(minutes=15),
            auto_fetch_if_empty=auto_fetch_if_empty
        )
        df_15m_closed = self._filter_closed_candles(df_15m, '15min', event_time)
        if df_15m_closed.empty:
            return None
        return float(df_15m_closed.iloc[-1]['close'])

    def _extract_post_window_15m(self, df_15m: pd.DataFrame, start_time: datetime, end_time: datetime) -> pd.DataFrame:
        """
        截取事件后的15min窗口数据，仅保留在 end_time 前已闭合的K线。
        """
        if df_15m.empty:
            return df_15m.copy()
        start_ts = pd.Timestamp(self._ensure_beijing_datetime(start_time))
        end_ts = pd.Timestamp(self._ensure_beijing_datetime(end_time))
        close_15m = pd.to_timedelta(15, unit='m')
        mask = (df_15m['datetime'] > start_ts) & ((df_15m['datetime'] + close_15m) <= end_ts)
        return df_15m.loc[mask].copy().sort_values('timestamp').reset_index(drop=True)

    def calculate_post_event_metrics(
        self,
        event_time: datetime,
        horizons: Optional[List[str]] = None,
        auto_fetch_if_empty: bool = True
    ) -> Dict[str, Any]:
        """
        计算事件后多周期统计指标（return / amplitude / volatility）。
        - return: horizon末收盘 / 事件参考价 - 1
        - amplitude: horizon窗口内 max(high)/min(low) - 1
        - volatility: horizon窗口内 bar-to-bar pct_change 的标准差
        """
        event_time = self._ensure_beijing_datetime(event_time)
        horizons = horizons or ['15min', '1h', '4h', '1d']
        valid_horizons = [h for h in horizons if h in TIMEFRAME_MINUTES]
        if not valid_horizons:
            raise ValueError("horizons 不能为空，且必须属于 15min/1h/4h/1d")

        reference_price = self._get_event_reference_price(event_time, auto_fetch_if_empty=auto_fetch_if_empty)
        result: Dict[str, Any] = {
            'event_time': event_time.isoformat(),
            'reference_price': reference_price,
            'metrics': {}
        }

        if reference_price is None or reference_price <= 0:
            for horizon in valid_horizons:
                result['metrics'][horizon] = {
                    'horizon_end': (event_time + timedelta(minutes=TIMEFRAME_MINUTES[horizon])).isoformat(),
                    'return': None,
                    'amplitude': None,
                    'volatility': None,
                    'sample_size': 0
                }
            return result

        max_minutes = max(TIMEFRAME_MINUTES[h] for h in valid_horizons)
        df_15m = self.load_data(
            '15min',
            start_time=event_time - timedelta(hours=2),
            end_time=event_time + timedelta(minutes=max_minutes + 30),
            auto_fetch_if_empty=auto_fetch_if_empty
        )
        df_15m = self._filter_closed_candles(
            df_15m,
            '15min',
            event_time + timedelta(minutes=max_minutes + 30)
        )

        for horizon in valid_horizons:
            horizon_end = event_time + timedelta(minutes=TIMEFRAME_MINUTES[horizon])
            window_df = self._extract_post_window_15m(df_15m, event_time, horizon_end)

            if window_df.empty:
                result['metrics'][horizon] = {
                    'horizon_end': horizon_end.isoformat(),
                    'return': None,
                    'amplitude': None,
                    'volatility': None,
                    'sample_size': 0
                }
                continue

            closes = window_df['close'].astype(float)
            highs = window_df['high'].astype(float)
            lows = window_df['low'].astype(float)
            close_returns = closes.pct_change().dropna()

            end_price = float(closes.iloc[-1])
            max_high = float(highs.max())
            min_low = float(lows.min())

            result['metrics'][horizon] = {
                'horizon_end': horizon_end.isoformat(),
                'end_price': end_price,
                'return': (end_price / reference_price) - 1.0,
                'amplitude': (max_high / min_low) - 1.0 if min_low > 0 else None,
                'volatility': float(close_returns.std(ddof=0)) if not close_returns.empty else 0.0,
                'sample_size': int(len(window_df))
            }

        return result

    def aggregate_post_event_metrics(
        self,
        event_metrics: List[Dict[str, Any]],
        horizons: Optional[List[str]] = None
    ) -> Dict[str, Dict[str, Any]]:
        """
        汇总多个事件的 post 指标，输出均值/中位数/胜率等统计。
        """
        horizons = horizons or ['15min', '1h', '4h', '1d']
        valid_horizons = [h for h in horizons if h in TIMEFRAME_MINUTES]
        summary: Dict[str, Dict[str, Any]] = {}

        for horizon in valid_horizons:
            rows: List[Dict[str, float]] = []
            for event_item in event_metrics:
                metric = event_item.get('metrics', {}).get(horizon, {})
                ret = metric.get('return')
                amp = metric.get('amplitude')
                vol = metric.get('volatility')
                if ret is None or amp is None or vol is None:
                    continue
                rows.append({
                    'return': float(ret),
                    'amplitude': float(amp),
                    'volatility': float(vol),
                })

            if not rows:
                summary[horizon] = {'sample_count': 0}
                continue

            df = pd.DataFrame(rows)
            summary[horizon] = {
                'sample_count': int(len(df)),
                'mean_return': float(df['return'].mean()),
                'median_return': float(df['return'].median()),
                'win_rate': float((df['return'] > 0).mean()),
                'mean_amplitude': float(df['amplitude'].mean()),
                'mean_volatility': float(df['volatility'].mean()),
            }

        return summary

    def get_latest_price(self) -> Dict:
        """获取合约ticker最新价格"""
        try:
            url = f"{BITGET_API_BASE}/ticker"
            params = {
                'symbol': CONTRACT_SYMBOL,
                'productType': CONTRACT_PRODUCT_TYPE,
            }
            data = self._get_json(url, params=params, timeout=10)

            if data.get('code') == '00000' and data.get('data'):
                ticker = data['data']
                if isinstance(ticker, list):
                    if not ticker:
                        return {}
                    ticker = ticker[0]

                def _to_float(*keys, default=0.0):
                    for key in keys:
                        value = ticker.get(key)
                        if value is None:
                            continue
                        try:
                            return float(value)
                        except (ValueError, TypeError):
                            continue
                    return float(default)

                return {
                    'symbol': ticker.get('symbol', CONTRACT_SYMBOL),
                    'product_type': CONTRACT_PRODUCT_TYPE,
                    'price': _to_float('lastPr', 'last'),
                    'change24h': _to_float('change24h'),
                    'high24h': _to_float('high24h'),
                    'low24h': _to_float('low24h'),
                    'volume24h': _to_float('baseVolume', 'baseVolume24h'),
                    'quote_volume24h': _to_float('quoteVolume', 'usdtVolume', 'quoteVolume24h'),
                    'timestamp': int(time.time() * 1000),
                    'datetime': datetime.now(CST).isoformat()
                }
        except Exception as e:
            print(f"获取最新价格失败: {e}")

        return {}

    def cleanup_old_data(self):
        """清理过期数据"""
        print("🧹 清理过期数据...")
        now = datetime.now(CST)

        for timeframe, config in TIMEFRAMES.items():
            cutoff_time = now - timedelta(days=config['retention_days'])
            cutoff_ts = self._to_ts_ms(cutoff_time)

            df = self._load_timeframe_dataframe(timeframe)
            if df.empty:
                continue

            before = len(df)
            df = df[df['timestamp'] >= cutoff_ts].copy().reset_index(drop=True)
            removed_count = before - len(df)
            if removed_count > 0:
                self._write_timeframe_dataframe(timeframe, df)
                print(f"  🗑️ {timeframe}: 删除 {removed_count} 条过期记录")

    def _split_time_range_to_batches(self, start_ts: int, end_ts: int, tf_ms: int, bar_limit: int):
        """
        辅助方法：将时间区间[start_ts, end_ts]切分成每批不超过bar_limit根K线的小区间（ms）
        返回值为[(batch_start_ms, batch_end_ms), ...]
        """
        batches = []
        batch_span_ms = bar_limit * tf_ms
        curr = start_ts
        while curr < end_ts:
            batch_end = min(curr + batch_span_ms - tf_ms, end_ts)
            batches.append((curr, batch_end))
            curr = batch_end + tf_ms
        return batches

    def _compute_missing_ranges_in_window(
        self,
        timestamps: List[int],
        window_start_ts: int,
        window_end_ts: int,
        tf_step_ms: int
    ) -> List[Tuple[int, int]]:
        """
        计算目标窗口内的缺失区间，仅返回 [window_start_ts, window_end_ts] 内缺口。
        """
        if window_end_ts < window_start_ts:
            return []

        window_timestamps = sorted(
            int(ts) for ts in timestamps if window_start_ts <= int(ts) <= window_end_ts
        )
        if not window_timestamps:
            return [(window_start_ts, window_end_ts)]

        missing_ranges: List[Tuple[int, int]] = []

        # 头部缺失
        if window_timestamps[0] > window_start_ts:
            ms = window_start_ts
            me = window_timestamps[0] - tf_step_ms
            if ms <= me:
                missing_ranges.append((ms, me))

        # 区间内部缺口
        for left, right in zip(window_timestamps[:-1], window_timestamps[1:]):
            if int(right) - int(left) > tf_step_ms:
                ms = int(left) + tf_step_ms
                me = int(right) - tf_step_ms
                if ms <= me:
                    missing_ranges.append((ms, me))

        # 尾部缺失
        if window_timestamps[-1] < window_end_ts:
            ms = window_timestamps[-1] + tf_step_ms
            me = window_end_ts
            if ms <= me:
                missing_ranges.append((ms, me))

        return missing_ranges

    def fetch_minimal_data(self, timeframe: str, start_ts: int, end_ts: int) -> int:
        """
        补齐指定时间范围内的本地K线数据，仅对缺失区间执行，避免冗余API请求。
        """
        candles = self.call_bitget_api(
            timeframe,
            start_ts=start_ts,
            end_ts=end_ts,
            limit=TIMEFRAMES[timeframe]['limit']
        )
        if candles:
            time.sleep(0.1)
            return candles
        else:
            return []

    def update_timeframe(
        self,
        timeframe: str,
        lookback_days: int = MIN_COVERAGE_DAYS,
        start_time: Optional[datetime] = None,
        end_time: Optional[datetime] = None
    ) -> int:
        """
        维护某时间框架的K线数据完整性，自动查找缺失区间并补全。
        1. 不存在数据文件则全量初始化下载
        2. 存在时，比对目标区间头/尾/中间所有缺口，分批补齐
        3. end_time 不允许超过当前时间
        """
        print(f"📈 更新 {timeframe} K线数据...")

        now = datetime.now(CST)
        end_time = self._ensure_beijing_datetime(end_time) if end_time else now
        if end_time > now:
            end_time = now
        start_time = (
            self._ensure_beijing_datetime(start_time)
            if start_time
            else end_time - timedelta(days=lookback_days)
        )
        if start_time > end_time:
            print(f"  [跳过] {timeframe} 开始时间晚于结束时间: {start_time} > {end_time}")
            return 0

        missing_ranges = []
        # 1. 初始化：文件不存在直接全量补全
        df = self._load_timeframe_dataframe(timeframe)
        tf_step_ms = self._timeframe_step_ms(timeframe)
        ts_min = int(self._to_ts_ms(start_time))
        ts_max = int(self._to_ts_ms(end_time))

        if df.empty or 'timestamp' not in df.columns:
            print(f"  [初始化] {timeframe} 本地数据文件不存在 or 无效，开始全量下载")
            missing_ranges.append((ts_min, ts_max))
        else:
            timestamps = df['timestamp'].astype('int64').tolist()
            missing_ranges = self._compute_missing_ranges_in_window(
                timestamps=timestamps,
                window_start_ts=ts_min,
                window_end_ts=ts_max,
                tf_step_ms=tf_step_ms
            )

        # 梳理所有缺区并拆分为API限制的小区间
        max_bars = TIMEFRAMES[timeframe]['limit']
        chunked_ranges = []
        for ms, me in missing_ranges:
            chunked_ranges.extend(self._split_time_range_to_batches(ms, me, tf_step_ms, max_bars))

        # 顺序补全所有实际缺失的小区间
        all_candles = []
        for ms, me in chunked_ranges:
            sdt = self._from_ts_ms(ms)
            edt = self._from_ts_ms(me)
            print(f"  [补全] {timeframe} 缺失 {sdt} ~ {edt}")
            candles = self.fetch_minimal_data(timeframe, ms, me)
            all_candles.extend(candles)
        self.save_candles(timeframe, all_candles)
        print(f"  💾 {timeframe} 更新拉取/补全 {len(all_candles)} 条K线")
        return len(all_candles)

    def update_timeframe_around_timepoint(
        self,
        timeframe: str,
        anchor_time: datetime,
        past_days: int = 45,
        future_days: int = 15
    ) -> int:
        """
        给定时间点，下载 [anchor_time-45天, anchor_time+15天] 的缺失数据。
        未来窗口不超过当前时间。
        """
        anchor_time = self._ensure_beijing_datetime(anchor_time)
        start_time = anchor_time - timedelta(days=past_days)
        end_time = min(anchor_time + timedelta(days=future_days), datetime.now(CST))
        print(f"🕒 {timeframe} 目标窗口: {start_time} ~ {end_time}")
        return self.update_timeframe(
            timeframe=timeframe,
            start_time=start_time,
            end_time=end_time
        )

    def update_all_around_timepoint(
        self,
        anchor_time: datetime,
        past_days: int = 45,
        future_days: int = 15,
        timeframes: Optional[List[str]] = None
    ) -> Dict[str, int]:
        """
        给定时间点，批量更新所有（或指定）时间框架：
        [anchor_time-past_days, anchor_time+future_days]，且 future 不超过当前时间。
        """
        selected = timeframes or list(TIMEFRAMES.keys())
        selected = [tf for tf in selected if tf in TIMEFRAMES]
        if not selected:
            raise ValueError("timeframes 不能为空，且必须属于 15min/1h/4h/1d")

        result: Dict[str, int] = {}
        for timeframe in selected:
            try:
                result[timeframe] = self.update_timeframe_around_timepoint(
                    timeframe=timeframe,
                    anchor_time=anchor_time,
                    past_days=past_days,
                    future_days=future_days
                )
            except Exception as e:
                print(f"❌ {timeframe} 更新失败: {e}")
                result[timeframe] = 0
        return result

    def update_all(self, lookback_days: int = MIN_COVERAGE_DAYS, clean_outdated_data: bool = False):
        """更新所有时间框架"""
        print(f"\n📊 BTC K线数据维护系统")
        print("=" * 50)

        total_candles = 0
        for timeframe in TIMEFRAMES.keys():
            try:
                count = self.update_timeframe(timeframe, lookback_days=lookback_days)
                total_candles += count
            except Exception as e:
                print(f"❌ {timeframe} 更新失败: {e}")

        print(f"\n✅ 数据更新完成！总共获取 {total_candles} 根K线")
        print(f"数据目录: {DATA_DIR}")

        # 清理过期数据
        if clean_outdated_data:
            self.cleanup_old_data()

        # 显示最新价格
        latest = self.get_latest_price()
        if latest:
            print(f"\n💰 BTC最新价格: ${latest['price']:,.0f} ({latest['change24h']:+.2f}%)")
            print(f"24h区间: ${latest['low24h']:,.0f} - ${latest['high24h']:,.0f}")

def main():
    manager = BTCKlineManager()

    # 更新所有时间框架数据
    manager.update_all(lookback_days=MIN_COVERAGE_DAYS)

    # 示例：查询最近24小时的1h数据
    print(f"\n📈 示例查询：最近24小时1h K线")
    print("-" * 30)
    recent_1h = manager.query_candles('1h', limit=5)

    if recent_1h:
        for candle in recent_1h[-5:]:  # 显示最新5根
            dt = datetime.fromtimestamp(candle['timestamp']/1000, tz=CST)
            print(f"{dt:%m-%d %H:%M} | O:{candle['open']:>8.0f} H:{candle['high']:>8.0f} L:{candle['low']:>8.0f} C:{candle['close']:>8.0f}")

if __name__ == "__main__":
    main()