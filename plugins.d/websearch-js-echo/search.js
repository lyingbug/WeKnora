// search(query, maxResults, includeDate, params) → [{title,url,snippet,content,source}]
// httpRequest({method,url,headers,body}) is provided by the host and is SSRF-safe.
function search(query, maxResults, includeDate, params) {
  var q = String(query || "").trim();
  if (!q || maxResults === 0) {
    return [];
  }
  return [
    {
      title: (params && params.api_key) ? "js-echo (keyed)" : "js-echo",
      url: "https://weknora.local/plugin/js-echo",
      snippet: q,
      content: q,
      source: "js-echo",
    },
  ];
}
