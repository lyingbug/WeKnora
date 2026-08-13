// Package all registers every built-in protocol driver.
//
// Importing it for its side effects is how a consumer opts into the standard
// protocols without naming each one, and it keeps the drivers themselves free
// of any dependency on each other.
package all

import (
	_ "github.com/Tencent/WeKnora/internal/models/llm/protocol/anthropic"
	_ "github.com/Tencent/WeKnora/internal/models/llm/protocol/openaichat"
	_ "github.com/Tencent/WeKnora/internal/models/llm/protocol/responses"
)
