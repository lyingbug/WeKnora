package plugin

// Well-known Context service keys. A seam is a swappable capability: the
// key names the Service Definition, a plugin Provides or Registers a
// Service Provider, and existing WeKnora services consume it.
//
// Keys stay stable so out-of-tree plugins can depend on them without
// importing a concrete implementation.
const (
	ServiceWebSearch = "web_search"
	ServiceRetriever = "retriever"
	ServiceStorage   = "storage"
	ServiceConnector = "datasource"
	ServiceIM        = "im"
	ServiceModel     = "model"
	ServiceChunker   = "chunker"
	ServiceParser    = "parser"
	ServiceAgentTool = "agent_tool"
	ServiceChat      = "chat_pipeline"
)

// Well-known event names. Dispatch mode is part of the public contract.
const (
	// EventPluginMounted is emitted after a plugin Apply succeeds. Mode: emit.
	EventPluginMounted = "plugin/mounted"
	// EventPluginUnloaded is emitted after a plugin context is closed. Mode: emit.
	EventPluginUnloaded = "plugin/unloaded"
)
