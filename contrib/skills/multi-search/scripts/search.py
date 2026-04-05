#!/usr/bin/env python3
"""
Multi-Search — One provider per invocation, agent picks the provider.

Usage:
    python3 search.py -p serper -q "query"
    python3 search.py --list-providers
"""

import argparse
import json
import os
import sys
from typing import NoReturn, Optional, List, Dict, Any
from urllib.request import Request, urlopen
from urllib.error import HTTPError, URLError
from urllib.parse import quote


# =============================================================================
# API Key Resolution (env vars only — no config.json, no .env files)
# =============================================================================

ENV_VAR_MAP = {
    "serper": "SERPER_API_KEY",
    "tavily": "TAVILY_API_KEY",
    "exa": "EXA_API_KEY",
    "you": "YOU_API_KEY",
}

SIGNUP_URLS = {
    "serper": "https://serper.dev",
    "tavily": "https://tavily.com",
    "exa": "https://exa.ai",
    "you": "https://api.you.com",
}


def get_api_key(provider: str) -> Optional[str]:
    """Read the API key from environment variables."""
    env_var = ENV_VAR_MAP.get(provider)
    if not env_var:
        return None
    return os.environ.get(env_var)


def require_api_key(provider: str) -> str:
    """Return the API key or exit with a helpful JSON error.

    Calls _die() (which calls sys.exit) if the key is missing or invalid,
    so the return type is always str when execution continues.
    """
    key = get_api_key(provider)
    env_var = ENV_VAR_MAP[provider]

    if not key:
        _die(
            {
                "error": f"Missing environment variable {env_var}",
                "provider": provider,
                "how_to_fix": f'export {env_var}="your-key"',
                "signup": SIGNUP_URLS.get(provider, ""),
            }
        )
        raise SystemExit(1)  # unreachable

    if len(key) < 10:
        _die(
            {
                "error": f"API key for {provider} appears invalid (too short)",
                "provider": provider,
            }
        )
        raise SystemExit(1)  # unreachable

    return key


def list_providers() -> Dict[str, Any]:
    """Return which providers are configured (have an env var set)."""
    providers = {}
    for prov, env_var in ENV_VAR_MAP.items():
        val = os.environ.get(env_var)
        providers[prov] = {
            "configured": bool(val),
            "env_var": env_var,
        }
    return providers


# =============================================================================
# HTTP helpers
# =============================================================================

USER_AGENT = "MultiSearch/1.0"


class _DieError(SystemExit):
    """Raised by _die — inherits from SystemExit so it terminates the process."""

    pass


def _die(error_obj: Dict[str, Any], code: int = 1) -> NoReturn:
    """Print JSON error to stderr and exit. Never returns."""
    print(json.dumps(error_obj, indent=2), file=sys.stderr)
    raise _DieError(code)


def make_request(
    url: str,
    headers: dict,
    body: Optional[dict] = None,
    method: str = "POST",
    timeout: int = 30,
) -> dict:
    """Make an HTTP request and return JSON response. Raises on failure."""
    if "User-Agent" not in headers:
        headers["User-Agent"] = USER_AGENT

    if body is not None:
        data = json.dumps(body).encode("utf-8")
    else:
        data = None

    req = Request(url, data=data, headers=headers, method=method)

    try:
        with urlopen(req, timeout=timeout) as response:
            return json.loads(response.read().decode("utf-8"))
    except HTTPError as e:
        error_body = e.read().decode("utf-8") if e.fp else str(e)
        try:
            error_json = json.loads(error_body)
            error_detail = (
                error_json.get("error") or error_json.get("message") or error_body
            )
        except json.JSONDecodeError:
            error_detail = error_body[:500]

        error_messages = {
            401: "Invalid or expired API key. Check your credentials.",
            403: "Access forbidden. Your API key may lack permissions.",
            429: "Rate limit exceeded. Wait and retry.",
            500: "Server error from the search provider.",
            503: "Service unavailable from the search provider.",
        }
        friendly_msg = error_messages.get(e.code, f"API error: {error_detail}")
        _die({"error": friendly_msg, "http_status": e.code, "detail": error_detail})
    except URLError as e:
        reason = str(getattr(e, "reason", e))
        _die({"error": f"Network error: {reason}"})
    except TimeoutError:
        _die({"error": f"Request timed out after {timeout}s"})
    except Exception as e:
        _die({"error": f"Unexpected error: {e}"})


# =============================================================================
# Provider: Serper (Google Search API)
# =============================================================================


def search_serper(
    query: str,
    api_key: str,
    max_results: int = 5,
    country: str = "us",
    language: str = "en",
    search_type: str = "search",
    time_range: Optional[str] = None,
    include_images: bool = False,
) -> dict:
    endpoint = f"https://google.serper.dev/{search_type}"

    body: Dict[str, Any] = {
        "q": query,
        "gl": country,
        "hl": language,
        "num": max_results,
        "autocorrect": True,
    }

    if time_range and time_range != "none":
        tbs_map = {
            "hour": "qdr:h",
            "day": "qdr:d",
            "week": "qdr:w",
            "month": "qdr:m",
            "year": "qdr:y",
        }
        if time_range in tbs_map:
            body["tbs"] = tbs_map[time_range]

    headers = {"X-API-KEY": api_key, "Content-Type": "application/json"}
    data = make_request(endpoint, headers, body)

    results = []
    for item in data.get("organic", [])[:max_results]:
        result: Dict[str, Any] = {
            "title": item.get("title", ""),
            "url": item.get("link", ""),
            "snippet": item.get("snippet", ""),
            "position": item.get("position"),
        }
        if item.get("date"):
            result["date"] = item["date"]
        if item.get("sitelinks"):
            result["sitelinks"] = item["sitelinks"]
        results.append(result)

    answer = ""
    if data.get("answerBox", {}).get("answer"):
        answer = data["answerBox"]["answer"]
    elif data.get("answerBox", {}).get("snippet"):
        answer = data["answerBox"]["snippet"]
    elif data.get("knowledgeGraph", {}).get("description"):
        answer = data["knowledgeGraph"]["description"]
    elif results:
        answer = results[0]["snippet"]

    images: List[str] = []
    if include_images:
        try:
            img_data = make_request(
                "https://google.serper.dev/images",
                headers,
                {"q": query, "gl": country, "hl": language, "num": 5},
            )
            images = [
                img.get("imageUrl", "")
                for img in img_data.get("images", [])[:5]
                if img.get("imageUrl")
            ]
        except SystemExit:
            pass  # make_request calls _die on failure; ignore for optional images

    return {
        "provider": "serper",
        "query": query,
        "results": results,
        "images": images,
        "answer": answer,
        "knowledge_graph": data.get("knowledgeGraph"),
        "related_searches": [r.get("query") for r in data.get("relatedSearches", [])],
    }


# =============================================================================
# Provider: Tavily (Research Search)
# =============================================================================


def search_tavily(
    query: str,
    api_key: str,
    max_results: int = 5,
    depth: str = "basic",
    topic: str = "general",
    include_domains: Optional[List[str]] = None,
    exclude_domains: Optional[List[str]] = None,
    include_images: bool = False,
    include_raw_content: bool = False,
) -> dict:
    endpoint = "https://api.tavily.com/search"

    body: Dict[str, Any] = {
        "query": query,
        "max_results": max_results,
        "search_depth": depth,
        "topic": topic,
        "include_images": include_images,
        "include_answer": True,
        "include_raw_content": include_raw_content,
    }
    if include_domains:
        body["include_domains"] = include_domains
    if exclude_domains:
        body["exclude_domains"] = exclude_domains

    headers = {
        "Content-Type": "application/json",
        "Authorization": f"Bearer {api_key}",
    }
    data = make_request(endpoint, headers, body)

    results = []
    for item in data.get("results", [])[:max_results]:
        result: Dict[str, Any] = {
            "title": item.get("title", ""),
            "url": item.get("url", ""),
            "snippet": item.get("content", ""),
            "score": round(item.get("score", 0.0), 3),
        }
        if include_raw_content and item.get("raw_content"):
            result["raw_content"] = item["raw_content"]
        results.append(result)

    return {
        "provider": "tavily",
        "query": query,
        "results": results,
        "images": data.get("images", []),
        "answer": data.get("answer", ""),
        "follow_up_questions": data.get("follow_up_questions"),
        "metadata": {
            "response_time": data.get("response_time"),
            "request_id": data.get("request_id"),
        },
    }


# =============================================================================
# Provider: Exa (Neural/Semantic Search)
# =============================================================================


def search_exa(
    query: str,
    api_key: str,
    max_results: int = 5,
    search_type: str = "neural",
    category: Optional[str] = None,
    start_date: Optional[str] = None,
    end_date: Optional[str] = None,
    similar_url: Optional[str] = None,
    include_domains: Optional[List[str]] = None,
    exclude_domains: Optional[List[str]] = None,
) -> dict:
    if similar_url:
        endpoint = "https://api.exa.ai/findSimilar"
        body: Dict[str, Any] = {
            "url": similar_url,
            "numResults": max_results,
            "contents": {"text": {"maxCharacters": 1000}, "highlights": True},
        }
    else:
        endpoint = "https://api.exa.ai/search"
        body = {
            "query": query,
            "numResults": max_results,
            "type": search_type,
            "contents": {"text": {"maxCharacters": 1000}, "highlights": True},
        }

    if category:
        body["category"] = category
    if start_date:
        body["startPublishedDate"] = start_date
    if end_date:
        body["endPublishedDate"] = end_date
    if include_domains:
        body["includeDomains"] = include_domains
    if exclude_domains:
        body["excludeDomains"] = exclude_domains

    headers = {"x-api-key": api_key, "Content-Type": "application/json"}
    data = make_request(endpoint, headers, body)

    results = []
    for item in data.get("results", [])[:max_results]:
        highlights = item.get("highlights", [])
        text = item.get("text", "")
        result: Dict[str, Any] = {
            "title": item.get("title", ""),
            "url": item.get("url", ""),
            "snippet": highlights[0] if highlights else text[:500],
            "score": round(item.get("score", 0.0), 3),
        }
        if len(highlights) > 1:
            result["highlights"] = highlights[1:]
        if text:
            result["text"] = text
        if item.get("publishedDate"):
            result["published_date"] = item["publishedDate"]
        if item.get("author"):
            result["author"] = item["author"]
        results.append(result)

    answer = results[0]["snippet"] if results else ""

    return {
        "provider": "exa",
        "query": query if not similar_url else f"Similar to: {similar_url}",
        "results": results,
        "images": [],
        "answer": answer,
    }


# =============================================================================
# Provider: You.com (LLM-Ready Web & News Search)
# =============================================================================


def search_you(
    query: str,
    api_key: str,
    max_results: int = 5,
    country: str = "US",
    language: str = "en",
    freshness: Optional[str] = None,
    safesearch: str = "moderate",
    include_news: bool = True,
    livecrawl: Optional[str] = None,
) -> dict:
    endpoint = "https://ydc-index.io/v1/search"

    params: Dict[str, Any] = {
        "query": query,
        "count": max_results,
        "safesearch": safesearch,
    }
    if country:
        params["country"] = country.upper()
    if language:
        params["language"] = language.upper()
    if freshness:
        params["freshness"] = freshness
    if livecrawl:
        params["livecrawl"] = livecrawl
        params["livecrawl_formats"] = "markdown"

    query_string = "&".join(f"{k}={quote(str(v))}" for k, v in params.items())
    url = f"{endpoint}?{query_string}"

    headers = {"X-API-KEY": api_key, "Accept": "application/json"}
    data = make_request(url, headers, method="GET")

    results_data = data.get("results", {})
    web_results = results_data.get("web", [])
    news_results = results_data.get("news", []) if include_news else []
    metadata = data.get("metadata", {})

    results = []
    for item in web_results[:max_results]:
        snippets = item.get("snippets", [])
        snippet = snippets[0] if snippets else item.get("description", "")

        result: Dict[str, Any] = {
            "title": item.get("title", ""),
            "url": item.get("url", ""),
            "snippet": snippet,
        }
        if len(snippets) > 1:
            result["additional_snippets"] = snippets[1:]
        if item.get("description") and item["description"] != snippet:
            result["description"] = item["description"]
        if item.get("page_age"):
            result["date"] = item["page_age"]
        if item.get("thumbnail_url"):
            result["thumbnail"] = item["thumbnail_url"]
        if item.get("contents"):
            result["raw_content"] = item["contents"].get("markdown") or item[
                "contents"
            ].get("html", "")
        results.append(result)

    news = []
    for item in news_results[:5]:
        news.append(
            {
                "title": item.get("title", ""),
                "url": item.get("url", ""),
                "snippet": item.get("description", ""),
                "date": item.get("page_age"),
                "thumbnail": item.get("thumbnail_url"),
                "source": "news",
            }
        )

    answer = ""
    if results:
        top_snippets = [r["snippet"] for r in results[:3] if r.get("snippet")]
        answer = " ".join(top_snippets)[:1000]

    return {
        "provider": "you",
        "query": query,
        "results": results,
        "news": news,
        "images": [],
        "answer": answer,
        "metadata": {
            "search_uuid": metadata.get("search_uuid"),
            "latency": metadata.get("latency"),
        },
    }


# =============================================================================
# CLI
# =============================================================================

ALL_PROVIDERS = ["serper", "tavily", "exa", "you"]


def main():
    parser = argparse.ArgumentParser(
        description="Multi-Search — search the web via one provider per invocation",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )

    # Required
    parser.add_argument(
        "-p", "--provider", choices=ALL_PROVIDERS, help="Search provider (required)"
    )
    parser.add_argument(
        "-q", "--query", help="Search query (required unless --similar-url)"
    )

    # Common
    parser.add_argument(
        "-n", "--max-results", type=int, default=5, help="Maximum results (default: 5)"
    )
    parser.add_argument(
        "--images", action="store_true", help="Include images (serper/tavily)"
    )
    parser.add_argument("--compact", action="store_true", help="Minified JSON output")

    # Meta
    parser.add_argument(
        "--list-providers",
        action="store_true",
        help="List providers and their configured status",
    )

    # Serper-specific
    parser.add_argument("--country", default="us")
    parser.add_argument("--language", default="en")
    parser.add_argument(
        "--type",
        dest="search_type",
        default="search",
        choices=["search", "news", "images", "videos", "places", "shopping"],
    )
    parser.add_argument(
        "--time-range", choices=["hour", "day", "week", "month", "year"]
    )

    # Tavily-specific
    parser.add_argument("--depth", default="basic", choices=["basic", "advanced"])
    parser.add_argument("--topic", default="general", choices=["general", "news"])
    parser.add_argument("--raw-content", action="store_true")
    parser.add_argument("--include-domains", nargs="+")
    parser.add_argument("--exclude-domains", nargs="+")

    # Exa-specific
    parser.add_argument("--exa-type", default="neural", choices=["neural", "keyword"])
    parser.add_argument(
        "--category",
        choices=[
            "company",
            "research paper",
            "news",
            "pdf",
            "github",
            "tweet",
            "personal site",
            "linkedin profile",
        ],
    )
    parser.add_argument("--start-date")
    parser.add_argument("--end-date")
    parser.add_argument("--similar-url")

    # You.com-specific
    parser.add_argument("--freshness", choices=["day", "week", "month", "year"])

    # You.com-specific
    parser.add_argument("--livecrawl", choices=["web", "news", "all"])
    parser.add_argument("--include-news", action="store_true", default=True)
    parser.add_argument(
        "--you-safesearch", default="moderate", choices=["off", "moderate", "strict"]
    )

    args = parser.parse_args()

    # --- Handle management commands (no provider/query needed) ---

    if args.list_providers:
        indent = None if args.compact else 2
        print(json.dumps(list_providers(), indent=indent, ensure_ascii=False))
        return

    # --- Validate required args ---

    if not args.provider:
        parser.error("-p/--provider is required")

    if not args.query and not args.similar_url:
        parser.error("-q/--query is required (unless using --similar-url with exa)")

    provider = args.provider

    # --- Get API key and execute search ---

    key = require_api_key(provider)

    result: Dict[str, Any]

    if provider == "serper":
        result = search_serper(
            query=args.query,
            api_key=key,
            max_results=args.max_results,
            country=args.country,
            language=args.language,
            search_type=args.search_type,
            time_range=args.time_range,
            include_images=args.images,
        )
    elif provider == "tavily":
        result = search_tavily(
            query=args.query,
            api_key=key,
            max_results=args.max_results,
            depth=args.depth,
            topic=args.topic,
            include_domains=args.include_domains,
            exclude_domains=args.exclude_domains,
            include_images=args.images,
            include_raw_content=args.raw_content,
        )
    elif provider == "exa":
        result = search_exa(
            query=args.query or "",
            api_key=key,
            max_results=args.max_results,
            search_type=args.exa_type,
            category=args.category,
            start_date=args.start_date,
            end_date=args.end_date,
            similar_url=args.similar_url,
            include_domains=args.include_domains,
            exclude_domains=args.exclude_domains,
        )
    elif provider == "you":
        result = search_you(
            query=args.query,
            api_key=key,
            max_results=args.max_results,
            country=args.country,
            language=args.language,
            freshness=args.freshness,
            safesearch=args.you_safesearch,
            include_news=args.include_news,
            livecrawl=args.livecrawl,
        )
    else:
        _die({"error": f"Unknown provider: {provider}"})

    # --- Output ---

    indent = None if args.compact else 2
    print(json.dumps(result, indent=indent, ensure_ascii=False))


if __name__ == "__main__":
    main()
