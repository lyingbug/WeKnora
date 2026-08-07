package router

import (
	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/handler"
)

// RegisterMemoryRoutes registers the long-term memory API.
//
// Authorisation here works differently from every other resource in this file,
// and the difference is deliberate.
//
// Personal memory routes carry no ownership guard because there is no resource
// identifier to guard. The service resolves the memory space from the request
// principal and refuses to accept one from the client, so a caller can only
// ever reach their own space; adding an OwnedXxxOrAdmin guard would imply an
// administrator override that must not exist. Viewer() is the floor purely to
// confirm the caller belongs to the workspace at all.
//
// The consequence is the important part: there is no endpoint, at any role,
// that returns another person's memories. Administrators get the workspace
// policy (tenant-settings) and anonymised aggregates (insights), never content.
//
// API keys are not granted access. Memory is tied to a human principal, and a
// workspace-scoped key has no single person whose memory it would be; the
// default-deny gate in rbac.go handles this by simply not declaring a policy.
func RegisterMemoryRoutes(r *gin.RouterGroup, memoryHandler *handler.MemoryHandler, g *rbacGuards) {
	personal := r.Group("/memory")
	{
		personal.GET("/space", g.Viewer(), memoryHandler.GetSpace)

		personal.GET("/settings", g.Viewer(), memoryHandler.GetSettings)
		personal.PUT("/settings", g.Viewer(), memoryHandler.UpdateSettings)

		// Workspace policy is administrator territory: it decides what every
		// member's memory may do.
		personal.GET("/tenant-settings", g.Admin(), memoryHandler.GetTenantSettings)
		personal.PUT("/tenant-settings", g.Admin(), memoryHandler.UpdateTenantSettings)

		personal.GET("/pages", g.Viewer(), memoryHandler.ListPages)
		personal.POST("/pages", g.Viewer(), memoryHandler.CreatePage)
		personal.GET("/pages/*slug", g.Viewer(), memoryHandler.GetPage)
		personal.PUT("/pages/*slug", g.Viewer(), memoryHandler.UpdatePage)
		personal.DELETE("/pages/*slug", g.Viewer(), memoryHandler.DeletePage)

		personal.GET("/search", g.Viewer(), memoryHandler.SearchPages)
		personal.GET("/revisions/*slug", g.Viewer(), memoryHandler.ListRevisions)
		personal.POST("/revert", g.Viewer(), memoryHandler.RevertPage)

		personal.GET("/notes", g.Viewer(), memoryHandler.ListNotes)
		personal.POST("/notes/:id/promote", g.Viewer(), memoryHandler.PromoteNote)
		personal.POST("/notes/:id/reject", g.Viewer(), memoryHandler.RejectNote)

		personal.GET("/graph", g.Viewer(), memoryHandler.GetGraph)
		personal.GET("/stats", g.Viewer(), memoryHandler.GetStats)

		personal.GET("/anchors", g.Viewer(), memoryHandler.ListAnchors)
		personal.POST("/anchors", g.Viewer(), memoryHandler.AddAnchor)
		personal.DELETE("/anchors/:id", g.Viewer(), memoryHandler.DeleteAnchor)

		personal.POST("/forget", g.Viewer(), memoryHandler.Forget)
		personal.GET("/export", g.Viewer(), memoryHandler.Export)
	}

	kbScoped := r.Group("/knowledgebase/:kb_id/memory")
	{
		// Coverage is the caller's own mastery of a knowledge base they can
		// read, so it needs the same KB access check as any other KB read.
		kbScoped.GET("/coverage", g.Viewer(), g.KBAccessRead("kb_id"), memoryHandler.GetCoverage)
		// Insights aggregate every member, so they are restricted to the
		// people responsible for the knowledge base's content.
		kbScoped.GET("/insights", g.OwnedWikiKBOrAdmin(), g.KBAccessRead("kb_id"), memoryHandler.GetInsights)
	}
}
