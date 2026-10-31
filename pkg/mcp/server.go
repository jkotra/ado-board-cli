package mcp

import (
	"fmt"

	"ado-board-cli/pkg/ado"
	"ado-board-cli/pkg/config"
	"ado-board-cli/pkg/db"
	"ado-board-cli/pkg/version"

	"github.com/mark3labs/mcp-go/server"
)

// NewServer creates a new MCPServer configured with ADO and App Control tools.
func NewServer(adoClient *ado.Client, database *db.DB, cfg *config.Config) *server.MCPServer {
	s := server.NewMCPServer(
		"ado-board-cli",
		version.Version,
		server.WithDescription("Azure DevOps Work Item Management and live ado-board-cli interactive TUI control server"),
		server.WithInstructions("Use this server to inspect and manage Azure DevOps work items, hierarchies, discussion comments, linked pull requests, and orchestrate the live ado-board-cli terminal application."),
		server.WithToolCapabilities(true),
	)

	// Register ADO tools and App Control tools
	RegisterAdoTools(s, adoClient, database, cfg)
	RegisterAppControlTools(s, adoClient, database)

	return s
}

// RunStdioServer starts the MCP server over standard input/output.
func RunStdioServer(adoClient *ado.Client, database *db.DB, cfg *config.Config) error {
	s := NewServer(adoClient, database, cfg)
	if err := server.ServeStdio(s); err != nil {
		return fmt.Errorf("MCP stdio server error: %w", err)
	}
	return nil
}
