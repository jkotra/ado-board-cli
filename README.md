# ⚡ ado-board-cli

[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org)
[![Bubble Tea](https://img.shields.io/badge/TUI-Bubble%20Tea-00D26A?style=flat)](https://github.com/charmbracelet/bubbletea)
[![Lipgloss](https://img.shields.io/badge/Styling-Lipgloss-FF5F87?style=flat)](https://github.com/charmbracelet/lipgloss)
[![Resty v3](https://img.shields.io/badge/REST-Resty%20v3-0052CC?style=flat)](https://github.com/go-resty/resty)
[![SQLite](https://img.shields.io/badge/Storage-SQLite%20(Pure%20Go)-003B57?style=flat&logo=sqlite)](https://modernc.org/sqlite)
[![Gemini](https://img.shields.io/badge/AI-Google%20Gemini-4285F4?style=flat&logo=google)](https://ai.google.dev)

**ado-board-cli** is a fast, keyboard-first Terminal User Interface (TUI) and CLI tool built in **Go** for managing, tracking, creating, editing, assigning, and estimating **Azure DevOps (ADO)** work items in a clean hierarchical tree view.

---

## 🌟 Key Features

- 🌲 **Recursive Tree Hierarchy**: Track any root item (Epic, Feature, Technical Enablement, Product Backlog Item, User Story) and automatically crawl its entire descendant tree.
- 📑 **Root Tabs Navigation**: Track multiple roots concurrently and switch between them using horizontal tabs directly above the bottom ribbon.
- 📊 **Dynamic Work & Progress Rollups**: Automatically aggregates original estimates, remaining work, completed work, and progress percentage (`6/12h (50%)` or `3/5 done`) up through all parent containers.
- 🤖 **Integrated Gemini AI Dock (`[g]`)**: Interactive conversational AI assistant capable of analyzing work item context, auto-assigning, updating statuses, summarizing discussions, and executing board actions.
- 👥 **Member Autocomplete & Marquee Scrolling**: Instant identity search with `@mention` resolution and smooth marquee scrolling for long titles and assignee names.
- 🔗 **Pull Request & Branch Discovery**: Discovers linked Azure DevOps Git pull requests with status badges (`[ACTIVE]`, `[COMPLETED]`, `[ABANDONED]`) and quick browser opening (`[o]`).
- ⚡ **Full CLI Command Suite**: Scriptable standalone commands (`track`, `untrack`, `sync`, `list`, `create`, `edit`, `status`, `assign`, `comment`, `delete`).
- 💾 **Pure-Go SQLite Cache**: CGO-free local storage (`~/.config/ado-board-cli/ado.db`) for instant startup and offline inspection.

---

## 🏗️ System Architecture

```
ado-board-cli/
├── cmd/               # Cobra CLI commands & subcommands
│   ├── root.go        # Root command & interactive TUI launcher
│   ├── track.go       # Track URL or ID
│   ├── untrack.go     # Remove root from tracking
│   ├── sync.go        # Full recursive refresh from Azure DevOps
│   ├── list.go        # Pretty-printed terminal hierarchy
│   ├── create.go      # CLI work item creation
│   ├── edit.go        # CLI field & estimate updater
│   ├── status.go      # CLI state changer
│   ├── assign.go      # CLI assignee updater
│   ├── comment.go     # CLI discussion commenter
│   ├── delete.go      # CLI work item deletion
│   ├── get.go         # Settings reader CLI
│   ├── set.go         # Settings writer CLI
│   └── mcp.go         # Model Context Protocol server & setup commands
├── pkg/
│   ├── ado/           # Azure DevOps REST API Client (Resty v3)
│   │   ├── client.go  # Work item crawler, JSON-patch updates, PR links, project members
│   │   └── types.go   # ADO payload models and parsing helpers
│   ├── config/        # Environment and configuration manager (PAT_TOKEN, GEMINI_API_KEY)
│   ├── db/            # Pure-Go SQLite storage engine
│   │   ├── db.go      # Schema migrations, CRUD queries, identity cache, settings
│   │   └── tree.go    # Hierarchical tree builder, rollups calculation, filtering, flattening
│   ├── gemini/        # Gemini AI Agent & Unified Tool Execution
│   │   ├── agent.go   # Gemini context builder, tool calling, and streaming executor
│   │   ├── executor.go# Shared execution engine for all ADO & app control tools
│   │   └── tools.go   # Function calling declarations and schemas
│   ├── ipc/           # Inter-Process Communication (Unix Domain Sockets / Named Pipes)
│   │   └── ipc.go     # IPC server, client bridge, protocol messages, and TUI telemetry
│   ├── mcp/           # Model Context Protocol (MCP) Server for Claude & AI Agents
│   │   ├── server.go  # MCP stdio server implementation (mcp-go)
│   │   ├── tools_ado.go   # 19 ADO work item & query management tools
│   │   ├── tools_app.go   # 8 Live TUI telemetry and screen orchestration tools
│   │   └── skill_content.go # Embedded self-contained SKILL.md documentation
│   └── tui/           # Charm Bubble Tea & Lipgloss TUI
│       ├── app.go           # Central TUI state machine and message router
│       ├── screen_tree.go   # Screen 1: Home hierarchical tree view with bottom tabs & filters
│       ├── screen_item.go   # Screen 2: Item details, PR links, discussion stream
│       ├── screen_create.go # Screen 3: Interactive work item creation form
│       ├── screen_settings.go # Screen 4: Application settings & preferences
│       ├── dock_gemini.go   # Gemini AI collapsible bottom dock with scrolling viewport
│       ├── ribbon.go        # Quick command shortcut ribbon and toast notifications
│       ├── autocomplete.go  # Identity & mention autocomplete engine
│       ├── modal_*.go       # Modals (Track, Filter, Status, Assign, Estimate, Schedule, Tab Manager)
│       └── styles.go        # Theme colors, badges, and layout primitives
├── build.sh           # Automated build, test & cross-compilation script
├── main.go            # Application entrypoint
└── README.md          # Project documentation
```

---

## 📦 Prerequisites & Setup

### 1. Requirements
- **Go 1.21+**
- **Azure DevOps Personal Access Token (PAT)** with **Work Items (Read & Write)** and **Code (Read & Write)** scopes (required for work item management, PR creation, attachments, reviewer assignment, and linking).
- *(Optional)* **Google Gemini API Key** for AI assistant features (`GEMINI_API_KEY`).

### 2. Authentication & Configuration
You can provide your Azure DevOps Personal Access Token (PAT) via SQLite database settings, environment variables, or CLI flags:

#### **Option A: Store in SQLite DB (Persistent)**
```bash
# Save PAT token persistently to local database
ado-board-cli set pat-token "your-azure-devops-pat-token"

# Clear database PAT to fallback to environment variables
ado-board-cli set pat-token ''
```

#### **Option B: Environment Variables (Fallback)**
Export credentials in your shell profile (`~/.bashrc`, `~/.zshrc`) or `.env`:

```bash
# Azure DevOps PAT Token (Fallback when DB is empty)
export PAT_TOKEN="your-azure-devops-pat-token"

# Optional: Set default organization and project if omitting URLs
export ADO_ORG="your-org-name"
export ADO_PROJECT="your-project-name"

# Optional: Google Gemini API Key for AI Dock ([g])
export GEMINI_API_KEY="AIzaSy..."
export GEMINI_MODEL="gemini-3.8-flash"  # Defaults to gemini-3.8-flash
```

*Resolution Order: `--token` flag > SQLite DB `pat-token` > `PAT_TOKEN` / `AZURE_DEVOPS_EXT_PAT` / `ADO_PAT` env vars.*

### 3. Build & Run
Use the included `build.sh` script to verify tests and compile optimized standalone binaries:

```bash
# Clone repository
git clone https://github.com/jkotra/ado-board-cli.git
cd ado-board-cli

# Build host Linux binary (output to ./bin/ado-board-cli)
./build.sh

# Build both Linux & Windows standalone binaries (./bin/ado-board-cli & ./bin/ado-board-cli.exe)
./build.sh --all

# Build Windows amd64 binary specifically with embedded icon & PE metadata (./bin/ado-board-cli.exe)
./build.sh --windows

# Build release binaries & distribution archives (.tar.xz for Linux, .zip for Windows in ./bin/)
./build.sh --release
```

---

## 🎯 Quick Start Guide

### 1. Track One or More Work Items
Track an Azure DevOps URL or work item ID:
```bash
./bin/ado-board-cli track "https://dev.azure.com/my-org/my-project/_workitems/edit/656842/"
./bin/ado-board-cli track "https://dev.azure.com/my-org/my-project/_workitems/edit/665688/"
```

### 2. Launch the Interactive TUI
```bash
# Terminal launch:
./bin/ado-board-cli

# On Windows: you can also double-click bin/ado-board-cli.exe in File Explorer!
```

### 3. Sync Board Data
```bash
./bin/ado-board-cli sync
```

---

## 🖥️ Interactive TUI Guide & Shortcuts Reference

`ado-board-cli` provides a keyboard-first, highly responsive terminal interface with full mouse support. Below is the complete reference of shortcuts and interactions across every screen, dock, and modal.

---

### **Screen 1: Home / Tree View**
Displays the active tracked root hierarchy with work item type badges, state indicators, assignees, estimates, and rollup progress.

| Category | Shortcut | Action | Description |
| :--- | :--- | :--- | :--- |
| **Navigation** | <kbd>↑</kbd> / <kbd>k</kbd> | Move Up | Move selection up one work item |
| | <kbd>↓</kbd> / <kbd>j</kbd> | Move Down | Move selection down one work item |
| | <kbd>Home</kbd> | Jump to Top | Jump directly to the first root item |
| | <kbd>End</kbd> | Jump to Bottom | Jump directly to the last visible item |
| | **Mouse Wheel** | Scroll | Scroll cursor up / down through the tree |
| | **Left Click** | Select Row | Click directly on any visible row to select it |
| **Hierarchy** | <kbd>Space</kbd> / <kbd>h</kbd> / <kbd>l</kbd> | Toggle Branch | Expand or collapse children of the selected item |
| | <kbd>Shift+Alt+Space</kbd> | Toggle Branch | Expand / collapse branch (works even when dock is open) |
| **Root Tabs** | <kbd>]</kbd> / <kbd>}</kbd> / <kbd>Tab</kbd> | Next Tab | Switch to the next tracked root tab |
| | <kbd>[</kbd> / <kbd>{</kbd> / <kbd>Shift+Tab</kbd> | Prev Tab | Switch to the previous tracked root tab |
| | <kbd>1</kbd> – <kbd>9</kbd> | Direct Jump | Switch directly to root Tab 1 through 9 |
| **Item Actions** | <kbd>Enter</kbd> | View Details | Open **Screen 2** (Item Details, PRs & Discussion) |
| | <kbd>o</kbd> | Open in Browser | Open selected work item in browser (`open-in-browser-cmd` / default) |
| | <kbd>Ctrl</kbd> + **Left Click** | Open in Browser | Click work item with Ctrl to open in browser |
| | <kbd>c</kbd> | Create Child | Open **Screen 3** to create a child under selected item |
| | <kbd>s</kbd> | Change Status | Open Status selector modal |
| | <kbd>Shift+S</kbd> | Settings | Open **Screen 4** (Settings & Preferences) |
| | <kbd>a</kbd> | Assign Member | Open Assignee selector modal with live autocomplete |
| | <kbd>v</kbd> / <kbd>V</kbd> / <kbd>Shift+A</kbd> | Activity | Open Activity selector modal (Development, Testing, Design, Deployment, Documentation, Requirements) |
| | <kbd>e</kbd> / <kbd>E</kbd> | Edit Estimates | Open Estimates & Effort modal (Effort, Original, Remaining, Completed) |
| | <kbd>d</kbd> / <kbd>S</kbd> | Schedule & Dates | Open interactive Calendar & Time Picker modal (Start Date, Target Date, Finish Date) |
| | <kbd>D</kbd> / <kbd>Shift+D</kbd> | Delete Item | Open Delete confirmation dialog |
| **Board, Filters & AI** | <kbd>t</kbd> | Tab Manager | Open Tab Menu to add roots/queries, rename tabs (<kbd>r</kbd>), or untrack |
| | <kbd>f</kbd> | Filter Board | Open filter modal (<kbd>Ctrl+S</kbd> = Global 🌐, <kbd>Ctrl+T</kbd> = Tab 📑, <kbd>c</kbd> = Clear Tab 📑) |
| | <kbd>F</kbd> | Clear All Filters | Instantly reset all global and tab-specific filters |
| | <kbd>i</kbd> | Incomplete Filter | Toggle quick filter for items missing estimates or story points |
| | <kbd>/</kbd> | Live Search | Search across titles, IDs, assignees, or states |
| | <kbd>g</kbd> | Gemini AI Dock | Open / toggle the Gemini AI conversational dock |
| | <kbd>r</kbd> | Sync Board | Refresh all tracked items from Azure DevOps |
| | <kbd>q</kbd> | Quit | Save UI state synchronously to database and exit |

---

### **Screen 2: Item Detail, PRs & Discussion**
Shows complete work item fields, formatted descriptions, acceptance criteria, estimate metrics, schedule dates, linked Git pull requests, and the full discussion thread.

| Category | Shortcut | Action | Description |
| :--- | :--- | :--- | :--- |
| **Viewport Scrolling** | <kbd>↑</kbd> / <kbd>k</kbd> | Scroll Up | Scroll viewport up one line |
| | <kbd>↓</kbd> / <kbd>j</kbd> | Scroll Down | Scroll viewport down one line |
| | <kbd>PgUp</kbd> / <kbd>Ctrl+u</kbd> | Page Up | Scroll viewport up one page / 4 lines |
| | <kbd>PgDn</kbd> / <kbd>Ctrl+d</kbd> | Page Down | Scroll viewport down one page / 4 lines |
| | **Mouse Wheel** | Scroll | Scroll viewport smoothly |
| **PRs & Links** | <kbd>[</kbd> / <kbd>]</kbd>, <kbd>Tab</kbd> / <kbd>Shift+Tab</kbd> | Cycle Links | Select / cycle through linked Pull Requests and branch links |
| | <kbd>p</kbd> / <kbd>Ctrl+Enter</kbd> / <kbd>Ctrl+J</kbd> | Open PR / Link | Open highlighted Pull Request or link in browser |
| | <kbd>o</kbd> | Open Work Item | Open the current work item in browser |
| | <kbd>Ctrl</kbd> + **Left Click** | Open Link | Click any link with Ctrl to open in browser |
| **Item Actions** | <kbd>c</kbd> | Add Comment | Open Comment modal with `@mention` autocomplete and markdown |
| | <kbd>e</kbd> | Edit Item | Edit title and description fields |
| | <kbd>s</kbd> | Change Status | Open Status selector modal |
| | <kbd>a</kbd> | Assign Member | Reassign work item to a team member |
| | <kbd>v</kbd> / <kbd>V</kbd> / <kbd>Shift+A</kbd> | Activity | Open Activity selector modal (Development, Testing, Design, Deployment, Documentation, Requirements) |
| | <kbd>w</kbd> / <kbd>E</kbd> | Edit Estimates | Edit Effort, Original Estimate, Remaining, and Completed work |
| | <kbd>d</kbd> / <kbd>S</kbd> | Schedule & Dates | Open interactive Calendar & Time Picker modal (Start, Target, Finish dates) |
| | <kbd>D</kbd> / <kbd>Shift+D</kbd> | Delete Item | Delete work item with confirmation dialog |
| | <kbd>g</kbd> | Gemini AI Dock | Open / toggle the Gemini AI conversational dock |
| | <kbd>r</kbd> | Refresh Item | Reload latest comments and details from Azure DevOps |
| | <kbd>Esc</kbd> / <kbd>b</kbd> | Back | Return to **Screen 1** (Tree View) |

---

### **Screen 3: Create Item Form**
Interactive form to create new work items linked directly to the selected parent container, with full date scheduling support and automatic tag inheritance based on configured tag regex.

| Shortcut | Action | Description |
| :--- | :--- | :--- |
| <kbd>Tab</kbd> / <kbd>↓</kbd> | Next Field | Move focus to the next input field (Title, Desc, Assignee, Estimate, Remaining, Start Date, Target Date, Finish Date) |
| <kbd>Shift+Tab</kbd> / <kbd>↑</kbd> | Previous Field | Move focus to the previous input field |
| <kbd>←</kbd> / <kbd>→</kbd> | Cycle Type | Change work item type (`Task`, `Bug`, `User Story`, `Feature`, `Epic`) |
| <kbd>Ctrl+S</kbd> / <kbd>Enter</kbd> | Submit | Create work item in Azure DevOps & local cache (when on submit button) |
| <kbd>Esc</kbd> | Cancel | Discard form and return to Tree View |

---

### **Screen 4: Settings Screen (`[Shift+S]`)**
Dedicated application settings screen to configure API keys, Azure DevOps authentication, model selection, tag inheritance regex, incomplete criteria, and work item estimation automation. Tokens are displayed in plain text for easy inspection.

| Category | Shortcut | Action | Description |
| :--- | :--- | :--- | :--- |
| **Navigation** | <kbd>Tab</kbd> / <kbd>↓</kbd> | Next Field | Cycle focus to the next field, checkbox, or button (auto-scrolls to focused element) |
| | <kbd>Shift+Tab</kbd> / <kbd>↑</kbd> | Previous Field | Cycle focus to the previous field, checkbox, or button (auto-scrolls to focused element) |
| **Scrolling** | <kbd>PgUp</kbd> / <kbd>PgDn</kbd> | Scroll Page | Scroll settings screen up or down |
| | <kbd>Ctrl+u</kbd> / <kbd>Ctrl+d</kbd> | Half Page | Scroll half page up or down |
| | Mouse Wheel | Smooth Scroll | Scroll settings view up or down with mouse wheel |
| **Configuration Fields** | | **PAT Token** | Azure DevOps Personal Access Token (stored in local database, plain text) |
| | | **Gemini API Key** | Google Gemini API key for AI assistant features (plain text) |
| | | **Gemini Model** | Selected Gemini model (e.g. `gemini-3.8-flash`, `gemini-2.5-pro`) |
| | | **Tag Regex Rules** | Comma/semicolon-separated regex rules for child tag inheritance & incomplete tag validation (e.g. `^BE-.*, ^FE-.*, ^release-.*`) |
| **Automation Checkboxes** | <kbd>Space</kbd> / <kbd>Enter</kbd> | **Compute Remaining Work Automatically** | Automatically calculates `Remaining Work = Original Estimate - Completed Work` |
| | <kbd>Space</kbd> / <kbd>Enter</kbd> | **Effort same as Estimate** | Automatically sets `Effort` to match `Estimate` unless custom Effort is entered |
| | <kbd>Space</kbd> / <kbd>Enter</kbd> | **Incomplete: Require Tag Matching Regex** | Flag work items as incomplete if no tag matches any configured `Tag Regex` rule |
| **Actions** | <kbd>Ctrl+S</kbd> / <kbd>Enter</kbd> | **Save Settings** | Persist settings to local SQLite database and update live runtime |
| | <kbd>Esc</kbd> / <kbd>Enter</kbd> | **Cancel** | Discard unsaved changes and return to Tree View |

---

### **Gemini AI Assistant Dock (`[g]`)**
A bottom dock powered by Google Gemini with live board context, multiline prompt editing, markdown rendering, tool execution, date scheduling, and background tree scrolling.

| Category | Shortcut | Action | Description |
| :--- | :--- | :--- | :--- |
| **Dock Controls** | <kbd>g</kbd> | Open Dock | Toggle Gemini AI Assistant dock open |
| | <kbd>Esc</kbd> / <kbd>Ctrl+C</kbd> | Hide Dock | Close dock and return full height to active screen |
| | <kbd>Ctrl+↑</kbd> | Expand Dock | Increase dock height by 2 rows |
| | <kbd>Ctrl+↓</kbd> | Shrink Dock | Decrease dock height by 2 rows |
| **Tree Navigation**<br>*(While Dock is Active)* | <kbd>Shift+↑</kbd> | Tree Up | Scroll cursor up in tree view above dock |
| | <kbd>Shift+↓</kbd> | Tree Down | Scroll cursor down in tree view above dock |
| | <kbd>Shift+Alt+Space</kbd> / <kbd>Alt+Space</kbd> | Tree Expand | Toggle branch expand / collapse in tree above dock |
| | <kbd>Alt+[</kbd> / <kbd>Alt+]</kbd> | Tree Tabs | Switch root tabs while typing in dock |
| | **Mouse Wheel** *(over tree)* | Scroll Tree | Scroll tree view directly with mouse |
| | **Left Click** *(over tree)* | Select Item | Click any tree row to select it |
| **Prompt Input** | <kbd>Enter</kbd> | Run Query | Send prompt to Gemini for analysis and tool execution |
| | <kbd>Shift+Enter</kbd> / <kbd>Alt+Enter</kbd><br><kbd>Ctrl+Enter</kbd> / <kbd>Ctrl+J</kbd> | Newline | Insert newline in prompt for multiline queries |
| | <kbd>↑</kbd> / <kbd>↓</kbd> | Prompt History | Navigate previous prompts (when prompt is empty or at first/last line) |
| | <kbd>Del</kbd> / <kbd>Ctrl+Del</kbd><br><kbd>Ctrl+L</kbd> / <kbd>Shift+Delete</kbd> | Clear History | Clear prompt input and reset Gemini conversation history |
| **Response Navigation** | <kbd>PgUp</kbd> / <kbd>PgDn</kbd> | Page Scroll | Scroll AI response page up / down |
| | <kbd>Ctrl+u</kbd> / <kbd>Ctrl+d</kbd> | Half Scroll | Scroll AI response up / down 4 lines |
| | <kbd>Home</kbd> / <kbd>End</kbd> | Top / Bottom | Jump to top / bottom of AI response (when prompt is empty) |
| | **Mouse Wheel** *(over dock)* | Scroll Response | Scroll AI response content |
| | <kbd>Ctrl</kbd> + **Left Click** | Open Link | Click any PR / work item URL in response to open in browser |

---

### **Modals & Dialogs Reference**

| Modal | Key | Shortcuts | Description |
| :--- | :--- | :--- | :--- |
| **Status Modal** | <kbd>s</kbd> | <kbd>↑</kbd>/<kbd>↓</kbd>, <kbd>1</kbd>–<kbd>6</kbd>, <kbd>Enter</kbd>, <kbd>Esc</kbd> | Select target state (`To Do`, `Active`, `In Progress`, `Resolved`, `Closed`, `Removed`) and apply |
| **Activity Modal** | <kbd>v</kbd> / <kbd>V</kbd> / <kbd>Shift+A</kbd> | <kbd>↑</kbd>/<kbd>↓</kbd>, <kbd>1</kbd>–<kbd>6</kbd>, <kbd>c</kbd> (Clear), <kbd>Enter</kbd>, <kbd>Esc</kbd> | Select activity category (`Development`, `Testing`, `Design`, `Deployment`, `Documentation`, `Requirements`) or clear |
| **Assign Modal** | <kbd>a</kbd> | <kbd>Type</kbd>, <kbd>Tab</kbd>, <kbd>↑</kbd>/<kbd>↓</kbd>, <kbd>Enter</kbd>, <kbd>Esc</kbd> | Search project members by name or email with live autocomplete |
| **Estimate Modal** | <kbd>e</kbd> / <kbd>E</kbd> / <kbd>w</kbd> | <kbd>Tab</kbd>, <kbd>Shift+Tab</kbd>, <kbd>Enter</kbd>, <kbd>Esc</kbd> | Edit Effort, Original Estimate, Remaining, and Completed work (auto-calculates remaining) |
| **Schedule & Dates Modal** | <kbd>d</kbd> / <kbd>S</kbd> | <kbd>Tab</kbd>, <kbd>1</kbd>/<kbd>2</kbd>/<kbd>3</kbd>, <kbd>←/→/↑/↓</kbd>, <kbd>t/m/w/x</kbd>, <kbd>Enter</kbd>, <kbd>Esc</kbd> | Interactive monthly calendar and time picker for Start Date, Target Date, and Finish Date |
| **Comment Modal** | <kbd>c</kbd> | <kbd>@</kbd> autocomplete, <kbd>Ctrl+S</kbd>, <kbd>Esc</kbd> | Multiline editor with `@mention` identity popup and markdown rendering |
| **Filter Modal** | <kbd>f</kbd> | <kbd>Tab</kbd>, <kbd>Space</kbd>, <kbd>Ctrl+S</kbd> (Global 🌐), <kbd>Ctrl+T</kbd> (Tab 📑), <kbd>c</kbd> (Clear Tab 📑), <kbd>Ctrl+R</kbd> (Clear All), <kbd>Esc</kbd> | Multi-criteria filtering by State, Assignee, Type with independent tab and global scoping |
| **Tab Manager** | <kbd>t</kbd> | <kbd>Tab</kbd>, <kbd>r</kbd> (Rename), <kbd>d</kbd>/<kbd>x</kbd> (Untrack), <kbd>Enter</kbd>, <kbd>Esc</kbd> | Add new root Azure DevOps URLs / IDs / Queries, rename tabs, or untrack roots |
| **Confirm Modal** | <kbd>D</kbd> / <kbd>Shift+D</kbd> | <kbd>y</kbd> / <kbd>Enter</kbd>, <kbd>n</kbd> / <kbd>Esc</kbd> | Confirm deletion of work item from Azure DevOps and local cache |

---

## 🛠️ CLI Scripting & Automation Reference

`ado-board-cli` supports direct command-line execution for CI/CD pipelines, scripts, and terminal power users:

```bash
# 1. Track & Untrack roots
ado-board-cli track "https://dev.azure.com/my-org/my-proj/_workitems/edit/656842"
ado-board-cli untrack 656842

# 2. View hierarchical tree in terminal (all tracked items or specific root)
ado-board-cli list
ado-board-cli list 656842

# 3. Full sync from Azure DevOps
ado-board-cli sync

# 4. Create a new work item with estimates, activity, and dates
ado-board-cli create \
  --type Task \
  --title "Implement OAuth2 Flow" \
  --parent 656842 \
  --activity "Development" \
  --assign "jane.doe@company.com" \
  --estimate 8 \
  --remaining 8 \
  --start-date "8/18/2026 12:00 AM" \
  --target-date "8/19/2026 8:01 AM" \
  --finish-date "8/21/2026 3:57 PM" \
  --desc "Add JWT token validation middleware"

# 5. Change state / status
ado-board-cli status 656842 "In Progress"

# 6. Assign item
ado-board-cli assign 656842 "john.doe@company.com"

# 7. Edit estimates, effort, activity, and schedule dates
ado-board-cli edit 656842 \
  --activity "Development" \
  --effort 8 \
  --estimate 16 \
  --remaining 8 \
  --completed 8 \
  --start-date "8/18/2026 12:00 AM" \
  --target-date "8/19/2026 8:01 AM" \
  --finish-date "8/21/2026 3:57 PM"

# 8. Clear specific schedule dates
ado-board-cli edit 656842 --clear-finish-date

# 8. Post a discussion comment (supports @mentions)
ado-board-cli comment 656842 "Initial implementation ready for review."

# 9. Delete work item
ado-board-cli delete 656842

# 10. Configure / inspect app settings
ado-board-cli set pat-token "your-azure-devops-pat"
ado-board-cli set gemini-model "gemini-3.8-flash"
ado-board-cli set open-in-browser-cmd 'google-chrome --profile-directory="Profile 1" "{url}"'
ado-board-cli get
```

---

## 🔌 Model Context Protocol (MCP) & AI Agent Integration

`ado-board-cli` includes a full-featured, standard stdio **Model Context Protocol (MCP)** server. This allows external AI assistants—such as **Claude Desktop**, **Claude Code CLI**, **DeepSeek Harness (`dsh`)**, **Google Antigravity**, and AI Coding Agents—to manage Azure DevOps work items and orchestrate your running interactive TUI.

### **1. Automated Setup**

Install and register `ado-board-cli` MCP server and agent skill files with one command:

```bash
# 📁 Local Workspace Setup (Default)
ado-board-cli mcp setup claude            # Claude Desktop & Claude Code CLI (.mcp.json & .claude.json)
ado-board-cli mcp setup agents            # Antigravity & AI Agents (.gemini/ & .agent/)
ado-board-cli mcp setup deepseek          # DeepSeek Harness (.dsh/cordis.patch.yml & SKILL.md)
ado-board-cli mcp setup opencode          # opencode (opencode.json & .opencode/skills/)

# 🌐 Global User Environment Setup
ado-board-cli mcp setup claude --global   # Claude Desktop & Claude Code CLI (~/.claude.json)
ado-board-cli mcp setup agents --global   # Antigravity & AI Agents (~/.gemini/config/)
ado-board-cli mcp setup deepseek --global # DeepSeek Harness (~/.dsh/cordis.patch.yml)
ado-board-cli mcp setup opencode --global # opencode (~/.config/opencode/)
```

### **2. MCP Helper Commands**

```bash
# View embedded SKILL.md instructions for AI agents
ado-board-cli mcp skill

# Print copy-pasteable JSON & YAML patch configuration for manual registration
ado-board-cli mcp config
```

### **3. Live TUI Telemetry & Bi-Directional Control**

When you have the `ado-board-cli` TUI open in a terminal, the MCP server automatically connects to it via local Inter-Process Communication (IPC Unix domain sockets or named pipes).

- **Cursor & Selection Awareness (`app_get_state` / `get_selected_item`)**: When you ask Claude or an AI Agent *"what item am I inspecting?"* or *"break down this user story into tasks"*, the AI immediately knows which item is currently highlighted under your cursor in the terminal.
- **Live Navigation & Filtering (`app_navigate_to_item`, `app_set_filter`, `app_clear_filter`)**: The AI can jump your terminal cursor directly to relevant items, expand parents, switch tabs, rename tabs, apply filters, or display popup toast notifications (`app_show_toast`).
- **Offline Resilience**: If the TUI is not running, all ADO data tools continue to function seamlessly via direct Azure DevOps REST APIs and local SQLite cache.

### **4. Available MCP Tools Reference**

| Category | Tool Name | Description |
|---|---|---|
| **ADO Operations** | `get_work_item_details` | Full item fields, parent/child links, discussions, and PR links |
| | `get_children` | Retrieves all children or recursive descendants under an item (Feature, Story, PBI, Epic, Bug) with hierarchy trees, rollup estimates, and status breakdowns |
| | `get_hierarchy` | Complete ancestor chain (from top root down) and descendant subtree hierarchy for a work item with rollup estimates |
| | `update_work_item` | Update state, estimates (original, remaining, completed, effort), activity, assignee, dates, tags, iteration, area, title, description |
| | `create_work_item` | Create new work item (Task, Bug, Story, Feature, Epic) with activity and automatic tag inheritance under any parent |
| | `batch_create_tasks` | Create multiple child tasks atomically with estimates, activity, and assignees |
| | `batch_update_items` | Bulk update states, estimates, activity, assignees, tags, and titles across items |
| | `delete_work_item` | Permanently delete a work item |
| | `add_comment` | Post discussion comments with markdown and @mentions |
| | `get_activity` | Chronological activity timeline (status changes, PRs, comments, estimates) |
| | `get_board_summary` | Summary of tracked roots, item counts, and rollup metrics |
| | `get_users` | Search team members and assignees for assignment suggestions |
| | `open_in_browser` | Open work items or queries in web browser |
| | `track_work_item` / `untrack_work_item` | Add or remove work item roots to/from the board |
| | `track_query` / `untrack_query` | Track or untrack Azure DevOps query URLs/IDs as board tabs |
| | `get_settings` / `update_settings` | Inspect or modify incomplete criteria (including tag regex & activity), tag regex pattern, sync settings, and auto-finish dates |
| **Pull Requests & Git** | `create_pull_request` | Create new PR from branch to target with reviewers, description, and linked work items (auto-detects repo) |
| | `get_pull_request_details` | Inspect complete PR metadata, reviewers/votes, status, linked work items, and attachments |
| | `get_pull_request_comments` | Retrieve discussion comments and review threads for a PR, including inline code comments |
| | `add_pull_request_comment` | Add PR review comment/thread with optional code line selection and markdown description |
| | `reply_pull_request_comment` | Reply to PR review comments and mark threads resolved/unresolved |
| | `get_pull_requests` | Linked Git pull requests for a work item with branch and approval status |
| | `update_pull_request` | Update PR title and markdown description (or append content/attachment links) |
| | `upload_pull_request_attachment` | Upload image or file attachment to a PR and return markdown embed snippet |
| | `add_pull_request_reviewers` | Add optional or required reviewers to a PR by display name, email, or GUID |
| | `link_pull_request_work_item` | Link a PR to a work item bidirectionally with ArtifactLink |
| **Live TUI Control** | `app_get_state` | Complete live context: cursor item, screen, active tab, tabs list, filters |
| | `get_selected_item` | Get the work item currently selected/highlighted in the running TUI |
| | `app_switch_tab` | Switch active tab on the running TUI screen |
| | `app_rename_tab` | Rename a board tab in the running TUI and local database |
| | `app_navigate_to_item` | Expand parents, highlight item, and optionally open detail view |
| | `app_refresh` | Trigger background sync and screen re-render across TUI windows |
| | `app_set_filter` | Apply global or tab-specific filters on the live screen |
| | `app_clear_filter` | Clear global, tab, or all active filters |
| | `app_show_toast` | Display an instant banner notification in the running TUI |
| | `app_set_screen` | Switch active screen (`tree`, `item`, `settings`, `create`) |

---

## ⚙️ Configuration & Settings Reference (`get` / `set`)

`ado-board-cli` maintains local configuration in the SQLite database `app_settings` table. Both `kebab-case` (`pat-token`) and `snake_case` (`pat_token`) key formats are supported.

### **Available Keys Reference Table**

| Setting Key | Description | Default Value | Fallback Environment Variable |
|---|---|---|---|
| `pat-token`<br>`pat_token` | Azure DevOps Personal Access Token (PAT) used for all ADO API calls | *(None / Empty)* | `PAT_TOKEN`<br>`AZURE_DEVOPS_EXT_PAT`<br>`ADO_PAT` |
| `gemini-api-key`<br>`gemini_api_key` | Google Gemini API Key for AI copilot chat, summaries, and actions | *(None / Empty)* | `GEMINI_API_KEY` |
| `gemini-model`<br>`gemini_model` | Active Gemini Model name for AI assistant queries | `gemini-3.8-flash` | `GEMINI_MODEL` |
| `tag-regex`<br>`tag_regex` | Comma/semicolon-separated regex rules for child tag inheritance & incomplete tag validation | `""` | `TAG_REGEX` |
| `open-in-browser-cmd`<br>`open_in_browser_cmd` | Command template to open work items/PRs in browser (`{url}` placeholder) | `""` *(OS Default)* | `OPEN_IN_BROWSER_CMD` |
| `default-org`<br>`default_org` | Default Azure DevOps organization when tracking numeric IDs | `my-org` | `ADO_ORG` |
| `default-project`<br>`default_project` | Default Azure DevOps project when tracking numeric IDs | `my-project` | `ADO_PROJECT` |

---

### **Precedence & Priority Order**
Settings are resolved in the following priority order (highest to lowest):
1. **Explicit CLI Flags** (e.g. `--token`, `--org`, `--project`, `--db`)
2. **SQLite Database Settings** (configured via `ado-board-cli set <key> <value>`)
3. **Environment Variables** (e.g. `PAT_TOKEN`, `GEMINI_API_KEY`, `GEMINI_MODEL`, `OPEN_IN_BROWSER_CMD`, `ADO_ORG`, `ADO_PROJECT`)
4. **Application Defaults** (e.g. `gemini-3.8-flash`, OS default browser)

---

### **Usage Examples for All Keys**

#### 1. `pat-token` (Azure DevOps Personal Access Token)
```bash
# Save PAT persistently in database (masked in output for security)
ado-board-cli set pat-token "your-ado-pat-token"

# View current PAT status (masked)
ado-board-cli get pat-token

# Clear DB value to fallback to PAT_TOKEN environment variable
ado-board-cli set pat-token ''
```

#### 2. `gemini-model` (AI Copilot Model)
```bash
# Switch to Gemini 3.8 Flash (Default)
ado-board-cli set gemini-model "gemini-3.8-flash"

# Switch to Gemini 2.5 Flash or Pro
ado-board-cli set gemini-model "gemini-2.5-flash"
ado-board-cli set gemini-model "gemini-2.5-pro"

# Inspect active model
ado-board-cli get gemini-model
```

#### 3. `gemini-api-key` (Google Gemini API Key)
```bash
# Store custom Gemini API key
ado-board-cli set gemini-api-key "AIzaSy..."

# View key status (masked)
ado-board-cli get gemini-api-key
```

#### 4. `open-in-browser-cmd` (Browser Integration)
```bash
# Google Chrome with specific profile
ado-board-cli set open-in-browser-cmd 'google-chrome --profile-directory="Profile 1" "{url}"'

# Google Chrome (default profile)
ado-board-cli set open-in-browser-cmd 'google-chrome "{url}"'

# Firefox / Brave / Edge
ado-board-cli set open-in-browser-cmd 'firefox "{url}"'
ado-board-cli set open-in-browser-cmd 'brave-browser "{url}"'
ado-board-cli set open-in-browser-cmd 'microsoft-edge "{url}"'

# Reset to system default browser (xdg-open / open / start)
ado-board-cli set open-in-browser-cmd ''

# View current browser setting
ado-board-cli get open-in-browser-cmd
```

#### 5. `default-org` & `default-project` (Azure DevOps Defaults)
```bash
# Set default organization
ado-board-cli set default-org "my-organization"

# Set default project
ado-board-cli set default-project "MyProject"

# View current defaults
ado-board-cli get default-org
ado-board-cli get default-project
```

#### 6. View All Configured Settings
```bash
# Print a formatted table of all settings stored in the database
ado-board-cli get
```

---

## 🌐 Browser Integration & Configuration

Pressing <kbd>o</kbd> in the TUI (from the Tree View or Item Details screen) opens the selected work item or linked Pull Request in a web browser.

### **Default Behavior**
By default, with no browser command configured, `ado-board-cli` automatically launches URLs in your **system default browser** using OS-native openers:
- **Linux**: `xdg-open <url>`
- **macOS**: `open <url>`
- **Windows**: `cmd /c start "" <url>`

---

## 💾 Local Storage & Database Schema

Local data is stored in `~/.ado-board-cli/ado.db` (or `./ado.db`) using pure-Go SQLite:
- `work_items`: Cached work item fields, titles, states, assignees, estimates, and relation pointers.
- `relations`: Parent-child and PR artifact links.
- `tracked_roots`: Registered root IDs, organizations, projects, and URLs.
- `identities`: Cached project members for instant autocomplete.
- `app_settings`: Configurable keys (e.g., active Gemini model, browser command, PAT token).

---

## 🧪 Testing & Quality Assurance

Run the comprehensive unit test suite covering ADO link parsing, tree hierarchy rollups, identity search, marquee scrolling, Gemini tool calling, and TUI view navigation:

```bash
go test -v ./...
```

---

## 🔒 Security & Privacy

- **Token Safety**: PAT tokens and API keys are read strictly from environment variables and never logged or committed.
- **Local Cache**: SQLite cache resides locally on your machine.
- **AI Privacy**: Context sent to Gemini is restricted to work item metadata necessary for prompt completion.

---

## 👤 Author
- **Jagadeesh Kotra** (<jagadeesh@stdin.top>)

---

## 📄 License

MIT License. Built with ❤️ for productive terminal workflows.
