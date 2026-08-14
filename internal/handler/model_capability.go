package handler

import (
	"net/http"
	"strings"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/models/llm/spi"
	_ "github.com/Tencent/WeKnora/internal/models/llm/vendors" // register the built-in model plugins
	"github.com/Tencent/WeKnora/internal/models/provider"
	secutils "github.com/Tencent/WeKnora/internal/utils"
	"github.com/gin-gonic/gin"
)

// GetModelCapabilities godoc
// @Summary      获取模型能力清单
// @Description  返回指定厂商+模型的参数能力清单，用于前端动态渲染模型配置表单
// @Tags         模型管理
// @Accept       json
// @Produce      json
// @Param        provider    query     string  true   "厂商标识，如 openai / aliyun / anthropic"
// @Param        model       query     string  false  "模型名称，用于命中模型级插件"
// @Param        model_type  query     string  false  "模型类型，默认 chat"
// @Param        protocol    query     string  false  "协议，厂商支持多协议时使用"
// @Success      200         {object}  map[string]interface{}  "能力清单"
// @Security     Bearer
// @Security     ApiKeyAuth
// @Router       /models/capabilities [get]
//
// The response is rendered from the same descriptor the request path uses, so
// the form can only offer controls that will actually reach the vendor. That
// is the point of the endpoint: the frontend used to predict this with its own
// copy of the provider rules, which had to be kept in sync by hand.
func (h *ModelHandler) GetModelCapabilities(c *gin.Context) {
	ctx := c.Request.Context()

	vendor := strings.TrimSpace(c.Query("provider"))
	model := strings.TrimSpace(c.Query("model"))
	baseURL := strings.TrimSpace(c.Query("base_url"))
	if vendor == "" && baseURL != "" {
		// A user who has pasted a base URL but not chosen a provider still
		// gets the right form, using the same detection the request path uses.
		vendor = string(provider.DetectProvider(baseURL))
	}
	if vendor == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "provider is required",
		})
		return
	}

	kind := modelKindFromQuery(c.Query("model_type"))
	logger.Infof(ctx, "Model capabilities requested: provider=%s model=%s kind=%s",
		secutils.SanitizeForLog(vendor), secutils.SanitizeForLog(model), kind)

	desc, ok := spi.Resolve(spi.Query{
		Vendor:   vendor,
		Kind:     kind,
		Model:    model,
		Protocol: spi.ProtocolID(strings.TrimSpace(c.Query("protocol"))),
	})
	if !ok {
		// A vendor with no plugin is a gap in the catalog, not an error: the
		// model still works through the generic transport, and the form should
		// fall back to its built-in defaults rather than break.
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    nil,
		})
		return
	}

	schema := desc.Schema()
	schema.Protocols = spi.Protocols(vendor, kind)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    schema,
	})
}

// modelKindFromQuery maps the frontend's model-type vocabulary onto the seam's.
func modelKindFromQuery(modelType string) spi.ModelKind {
	switch strings.ToLower(strings.TrimSpace(modelType)) {
	case "embedding":
		return spi.KindEmbedding
	case "rerank":
		return spi.KindRerank
	case "vllm", "vision":
		return spi.KindVision
	case "asr":
		return spi.KindASR
	default:
		return spi.KindChat
	}
}
