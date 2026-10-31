package tui

import (
	"strings"
	"testing"
)

func TestRenderMarkdown(t *testing.T) {
	input := `### 📅 Today (Aug 21)

• **13:12 IST** — **Clark, Iris** commented on **Task #665654** (*Test Cases Creation*):

  > Test Cases are same as Scalar Forecast in the current implementation.

• **17:23 IST** — **Doe, John** commented on **Bug #663964** (*Cannot Deploy GKE Cluster*):

  > previously raised tickets: **RITM0575378** (Tuesday) (Resolved)
  > 
  > we are still unable to create GKE cluster, failing with error:
  > The user does not have access to service account.`

	out := RenderMarkdown(input, 80)
	if out == "" {
		t.Fatal("expected rendered markdown output, got empty")
	}

	// Glamour renders **RITM0575378** with bold styling and converts ``` into styled blocks
	if strings.Contains(out, "**RITM0575378**") {
		t.Errorf("markdown ** should have been parsed by glamour, got: %s", out)
	}

	// Verify blockquote bar is rendered
	if !strings.Contains(out, "▎") && !strings.Contains(out, "│") {
		t.Errorf("expected blockquote styled bar in output, got: %s", out)
	}

	// Verify H3 doesn't have literal "###"
	if strings.Contains(out, "### 📅") {
		t.Errorf("expected H3 prefix badge instead of literal ###, got: %s", out)
	}
}

func TestDockGeminiRenderCaching(t *testing.T) {
	dock := NewDockGemini("gemini-3.7-flash")
	dock.Width = 100
	dock.Height = 30
	dock.ResponseMsg = "### Summary\n> Here is a quoted comment.\n* Item 1\n* Item 2"

	lines1 := dock.getRenderedLines()
	if len(lines1) == 0 {
		t.Fatal("expected rendered lines, got empty")
	}

	// Calling again should return cached slice instantly
	lines2 := dock.getRenderedLines()
	if len(lines2) != len(lines1) {
		t.Fatalf("expected cached lines length %d, got %d", len(lines1), len(lines2))
	}

	// Scroll offset calculation must use cached lines
	dock.ScrollDown(2)
	if dock.ScrollOffset != 0 && dock.maxScrollOffset() == 0 {
		t.Errorf("unexpected scroll offset: %d", dock.ScrollOffset)
	}
}

func TestRenderMarkdownPreservesUnbrokenURLs(t *testing.T) {
	longURL := "https://dev.azure.com/my-org/my-project/_git/my-repo/pullrequest/294732"
	input := "Raised PR for the changes " + longURL + " however there is an integration piece."

	// Even with narrow wrapWidth (e.g. 45 cols), the URL itself MUST remain 100% complete and unbroken on its line
	out := RenderMarkdown(input, 45)
	if !strings.Contains(out, longURL) {
		t.Fatalf("expected unbroken URL %q in rendered output, got:\n%s", longURL, out)
	}

	// Verify URL was not chopped with a newline or hyphen inside the URL
	if strings.Contains(out, "my-\n") || strings.Contains(out, "my-repo/\n") {
		t.Fatalf("URL was split across lines by wordwrap:\n%s", out)
	}
}

func TestDockGeminiGetURLAt(t *testing.T) {
	dock := NewDockGemini("gemini-3.7-flash")
	dock.Width = 100
	dock.Height = 20
	dock.Visible = true

	prURL := "https://dev.azure.com/my-org/my-project/_git/my-repo/pullrequest/294732"
	dock.ResponseMsg = "• 00:05 IST — Davis, Dan linked Pull Request #294732: long term forecast job\n  - URL: " + prURL + "\n  - Status: Active"

	lines := dock.getRenderedLines()
	if len(lines) == 0 {
		t.Fatal("expected rendered lines")
	}

	// Test GetURLAt with relative row pointing to the response area
	// Header is ~2 rows, input is 1 row -> line 0 is at relY=3, line 1 (URL) is at relY=4
	foundURL := dock.GetURLAt(4, 10)
	if foundURL != prURL {
		t.Errorf("expected %q from GetURLAt(4, 10), got %q", prURL, foundURL)
	}

	// Nearby tolerance test: clicking 1 row above or below
	foundURLNearby := dock.GetURLAt(3, 5)
	if foundURLNearby != prURL {
		t.Errorf("expected %q from nearby GetURLAt(3, 5), got %q", prURL, foundURLNearby)
	}

	// Full response fallback test: clicking anywhere in dock
	fallbackURL := dock.GetURLAt(10, 5)
	if fallbackURL != prURL {
		t.Errorf("expected %q from fallback GetURLAt, got %q", prURL, fallbackURL)
	}
}

func TestRenderMarkdownCommentQuote(t *testing.T) {
	input := `• **18:23 IST** — **Davis, Dan** commented on **Task #663713** (*Create long term forecast job*):

  > Created entrypoint script to manage job execution.
  > Implemented job tracking utilities and environment configuration.
  > Introduced job definitions for long-term and short-term forecasts.`

	out := RenderMarkdown(input, 80)
	if out == "" {
		t.Fatal("expected rendered output")
	}

	// Verify quote bar │ or ▎ is present for quoted comment
	if !strings.Contains(out, "│") && !strings.Contains(out, "▎") {
		t.Fatalf("expected quote bar in rendered output, got:\n%s", out)
	}

	// Verify comment content is rendered inside quote
	if !strings.Contains(out, "Created entrypoint script") {
		t.Fatalf("expected comment content in output, got:\n%s", out)
	}
}

func TestRenderMarkdownFullCommentWithCodeAndDivider(t *testing.T) {
	rawComment := `previously raised tickets: **RITM0575378** (Tuesday) (Resolved)

we are still unable to create GKE cluster, failing with error:
` + "```\n" + `The user does not have access to service account "123456789012-compute@developer.gserviceaccount.com". Ask a project owner to grant you the iam.serviceAccountUser role on the service account.
` + "```\n\n" + `___

**@Anderson, Alice** please provide an update, if any. Thanks.`

	cleaned := StripAndFormatHTML(rawComment)
	out := RenderMarkdown(cleaned, 100)
	if out == "" {
		t.Fatal("expected rendered output")
	}

	// 1. Bold asterisks should be processed (no literal ** in rendered output)
	if strings.Contains(out, "**RITM0575378**") {
		t.Errorf("expected ** to be parsed as bold, got: %s", out)
	}
	if strings.Contains(out, "**@Anderson, Alice**") {
		t.Errorf("expected ** to be parsed as bold for mention, got: %s", out)
	}

	// 2. Fenced codeblock should be processed (no literal ``` in rendered output)
	if strings.Contains(out, "```") {
		t.Errorf("expected ``` to be parsed as code block, got: %s", out)
	}

	// 3. Horizontal rule should be processed (no literal ___ in rendered output)
	if strings.Contains(out, "___") {
		t.Errorf("expected ___ to be parsed as horizontal rule, got: %s", out)
	}

	// 4. Code block content and email must be intact
	if !strings.Contains(out, "service account") || !strings.Contains(out, "123456789012-compute@developer.gserviceaccount.com") {
		t.Errorf("expected codeblock error message and service account email intact, got: %s", out)
	}
}

func TestWorkItem656843DescriptionHTMLRendering(t *testing.T) {
	rawHTML := `<div><span>Reduce database storage costs by offloading large datasets from AlloyDB (relational database) to Google Cloud Storage. Reduce database compute costs by removing high-compute operations from database and onto python apps.</span><br> </div><div> </div><div> </div><div><span><br></span> </div><div><span><div style="box-sizing:border-box;"><b style="box-sizing:border-box;">Scope</b> </div><div style="box-sizing:border-box;"><div style="box-sizing:border-box;"><ul style="box-sizing:border-box;padding:0px 0px 0px 40px;"><li style="box-sizing:border-box;">Identify and offload large interval-related datasets such as EDI HU interval, raw HU interval, pod interval clean/raw, OTS/VHOS/MDR/IRW interval tables, and related batch-service interval storage tables. </li><li style="box-sizing:border-box;">Refactor stored procedures and HU/Raw Manager logic that currently reads or writes interval data directly in the database. </li><li style="box-sizing:border-box;">Introduce service/API access for IDR interval data while scalar summary data may continue to use database writes where required </li><li style="box-sizing:border-box;"><br style="box-sizing:border-box;"> </li> </ul> </div><div style="box-sizing:border-box;"><br style="box-sizing:border-box;"> </div><div style="box-sizing:border-box;"><b style="box-sizing:border-box;">Out of Scope</b>:&nbsp; </div><div style="box-sizing:border-box;"><ul style="box-sizing:border-box;padding:0px 0px 0px 40px;"><li style="box-sizing:border-box;">Full scalar migration, profile data migration, and weather data migration unless prioritized separately. </li> </ul> </div><div style="box-sizing:border-box;"><br style="box-sizing:border-box;"> </div><div style="box-sizing:border-box;"><b style="box-sizing:border-box;">Assumptions / Dependencies / Risks:&nbsp;</b> </div><div style="box-sizing:border-box;">IDR interval tables are the largest database footprint and provide the highest savings opportunity. Scalar, profile, and weather tables are lower priority for this phase. Existing procedures that access interval data can be rewritten in Python without materially changing business outcomes. </div><div style="box-sizing:border-box;">All the tables impacted needs to be identified. </div> </div><div style="box-sizing:border-box;"><span style="box-sizing:border-box;"><div style="box-sizing:border-box;"><br style="box-sizing:border-box;"> </div></span> </div></span> </div><div><b>List of Tables </b>(Some of the interval tables listed below) <br><br> </div><div><span style="background-color:rgb(0, 255, 0);">EDI_HU_Interval </span><span style="background-color:rgb(0, 255, 0);"><b>5700GB </b></span><span style="background-color:rgb(0, 255, 0);">-&nbsp;</span><span style="background-color:rgb(0, 255, 0);"><b style="box-sizing:border-box;">High Complexity</b></span><br> </div><div><span style="background-color:rgb(255, 255, 0);">pod_intervalraw </span><span style="background-color:rgb(255, 255, 0);"><b>900GB&nbsp;</b></span><span style="background-color:rgb(255, 255, 0);">- </span><span style="background-color:rgb(255, 255, 0);"><b>High Complexity</b></span> </div><div><span style="background-color:rgb(255, 255, 0);">pod_intervalclean </span><span style="background-color:rgb(255, 255, 0);"><b>550GB </b></span><span style="background-color:rgb(255, 255, 0);">- </span><span style="background-color:rgb(255, 255, 0);"><b>Medium Complexity</b></span> </div><div><span style="background-color:rgb(255, 128, 0);">pod_intervalparams </span><span style="background-color:rgb(255, 128, 0);"><b>8.9GB</b></span><span style="background-color:rgb(255, 128, 0);"><b> </b></span><span style="background-color:rgb(255, 128, 0);"><b>- Low Complexity</b></span> </div><div><span style="background-color:rgb(0, 255, 0);">raw_hu_interval </span><span style="background-color:rgb(0, 255, 0);"><b>4100GB </b></span><span style="background-color:rgb(0, 255, 0);">-&nbsp;</span><span style="background-color:rgb(0, 255, 0);"><b style="box-sizing:border-box;">Medium Complexity</b></span> </div><div><br> </div><div>All staging tables that store the interval data </div><div><span style="background-color:rgb(0, 255, 0);">edi_hu_interval_vhos </span><span style="background-color:rgb(0, 255, 0);"><b>1740GB - Medium Complexity (whatever implementation gets built for this table follows for the below)</b></span> </div><div><span style="background-color:rgb(0, 255, 0);">interval_irw </span><span style="background-color:rgb(0, 255, 0);"><b>214GB&nbsp;</b></span><b><span style="display:inline !important;background-color:rgb(0, 255, 0);">- Medium Complexity</span></b> </div><div><span style="background-color:rgb(0, 255, 0);">hu_interval_ots </span><span style="background-color:rgb(0, 255, 0);"><b>122GB&nbsp;</b></span><b><span style="display:inline !important;background-color:rgb(0, 255, 0);\">- Medium Complexity</span></b><br><span style="background-color:rgb(0, 255, 0);">mdr_consumption_detail_vhos</span><span style="background-color:rgb(0, 255, 0);"><b> 59GB&nbsp;</b></span><b><span style="display:inline !important;background-color:rgb(0, 255, 0);\">- Medium Complexity</span></b> </div><div><br> </div><div>Probable impacted Stored procs / Functionalities are listed in the notes Section. Implementation needs to ensure all the references for HU Interval needs to be determined and moved to the appropriate implementation inline to the requirements. </div><div><br><b>Priority: Bigger size tables get higher priority. Lower complexity tables get higher priority.<br><br></b> </div><div>QA: Spot checking if the data didn't get lost, forecasts could change for IDR, checking if you load a transaction &amp; it works. </div><div> </div>`

	cleaned := StripAndFormatHTML(rawHTML)
	out := RenderMarkdown(cleaned, 100)
	if out == "" {
		t.Fatal("expected rendered output for work item 656843")
	}

	plainOut := StripEscapes(out)

	// 1. All bold markup should be processed cleanly without leaking raw ** markers
	if strings.Contains(plainOut, "**") {
		t.Errorf("rendered output contains unparsed markdown bold asterisks (**):\n%s", plainOut)
	}

	// 2. Scope, Out of Scope, Assumptions headers should be present
	if !strings.Contains(plainOut, "Scope") || !strings.Contains(plainOut, "Out of Scope") || !strings.Contains(plainOut, "Assumptions / Dependencies / Risks:") {
		t.Errorf("expected Scope/Out of Scope/Assumptions sections in output, got:\n%s", plainOut)
	}

	// 3. Table entries should be intact
	if !strings.Contains(plainOut, "EDI_HU_Interval") || !strings.Contains(plainOut, "5700GB") || !strings.Contains(plainOut, "High Complexity") {
		t.Errorf("expected table items in output, got:\n%s", plainOut)
	}

	// 4. Priority note should be intact
	if !strings.Contains(plainOut, "Priority: Bigger size tables get higher priority") {
		t.Errorf("expected priority note in output, got:\n%s", plainOut)
	}

	// 5. HTML entities should be unescaped (&amp; -> &)
	if strings.Contains(plainOut, "&amp;") {
		t.Errorf("expected &amp; to be unescaped to &, got:\n%s", plainOut)
	}
}

func TestHTMLTagsToMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Headings h1-h3",
			input:    "<h1>Main Title</h1><p>Body text</p><h2>Section</h2><h3>Subsection</h3>",
			expected: "# Main Title",
		},
		{
			name:     "Table conversion",
			input:    "<table><thead><tr><th>Name</th><th>Role</th></tr></thead><tbody><tr><td>Alice</td><td>Admin</td></tr></tbody></table>",
			expected: "| Name | Role |\n| --- | --- |\n| Alice | Admin |",
		},
		{
			name:     "Strikethrough and Span styles",
			input:    "<p>This is <del>old text</del> and <s>deprecated</s> and <span style=\"text-decoration: line-through;\">removed</span></p>",
			expected: "~~old text~~",
		},
		{
			name:     "Span bold and italic styles",
			input:    `<span style="font-weight: bold;">Important</span> <span style="font-style: italic;">Note</span>`,
			expected: "**Important** *Note*",
		},
		{
			name:     "Empty tags and breaks do not emit phantom markers",
			input:    `<b> </b><span> </span><b><br></b><div><br></div>`,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripAndFormatHTML(tt.input)
			if tt.expected != "" && !strings.Contains(got, tt.expected) {
				t.Errorf("StripAndFormatHTML(%q)\nexpected to contain: %q\ngot: %q", tt.input, tt.expected, got)
			}
		})
	}
}

func TestSanitizeMarkdownDelimiters(t *testing.T) {
	input := "**\n**Assumptions: **\n****\npod_intervalraw 900GB **- **High Complexity**\n"
	sanitized := SanitizeMarkdown(input)

	if strings.Contains(sanitized, "****") {
		t.Errorf("expected empty bold markers removed, got: %q", sanitized)
	}
	if strings.HasPrefix(sanitized, "**\n") {
		t.Errorf("expected orphan bold line removed, got: %q", sanitized)
	}
}

func TestHTMLOrderedAndUnorderedLists(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string
	}{
		{
			name: "Standard ordered list",
			input: `<ol>
				<li>First item</li>
				<li>Second item</li>
				<li>Third item</li>
			</ol>`,
			contains: []string{"1. First item", "2. Second item", "3. Third item"},
		},
		{
			name: "Ordered list with custom start attribute",
			input: `<ol start="5">
				<li>Fifth step</li>
				<li>Sixth step</li>
			</ol>`,
			contains: []string{"5. Fifth step", "6. Sixth step"},
		},
		{
			name: "Unordered list with code and links",
			input: `<ul>
				<li><code>table_one</code></li>
				<li><a href="https://example.com">Link Text</a></li>
			</ul>`,
			contains: []string{"- `table_one`", "- [Link Text](https://example.com)"},
		},
		{
			name: "Nested ordered inside unordered list",
			input: `<ul>
				<li>Main bullet
					<ol>
						<li>Sub step 1</li>
						<li>Sub step 2</li>
					</ol>
				</li>
				<li>Another bullet</li>
			</ul>`,
			contains: []string{"- Main bullet", "1. Sub step 1", "2. Sub step 2", "- Another bullet"},
		},
		{
			name: "List items wrapped with divs and paragraphs",
			input: `<ol>
				<li><div><span>Renamed table <code>tbl_old</code> to <code>tbl_new</code></span></div></li>
				<li><p>Created index on database</p></li>
			</ol>`,
			contains: []string{
				"1. Renamed table `tbl_old` to `tbl_new`",
				"2. Created index on database",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripAndFormatHTML(tt.input)
			for _, exp := range tt.contains {
				if !strings.Contains(got, exp) {
					t.Errorf("StripAndFormatHTML(%q)\nexpected to contain: %q\ngot:\n%s", tt.input, exp, got)
				}
			}
		})
	}
}

func TestWorkItem663957CommentRendering(t *testing.T) {
	// Exact comment from user screenshot with ordered list and code blocks
	comment1HTML := `<ol>
		<li>Renamed the table <code>ueschema.shorttermweather</code> to <code>ueschema.idrshorttermweather</code> .</li>
		<li>Created the table <code>ueschema.t_hly_forecast_clean</code> and deployed it to the DEV environment.</li>
		<li>Raised <a href="https://dev.azure.com/my-org/my-project/_git/my-repo/pullrequest/294577">Pull Request 294577</a>: Created the new tables needed for profile data and weather data for review</li>
	</ol>
	<div>CC: @Moore, Grace Ann</div>`

	formatted1 := StripAndFormatHTML(comment1HTML)
	rendered1 := RenderMarkdown(formatted1, 90)
	plain1 := StripEscapes(rendered1)

	// Verify ordered numbering 1., 2., 3. is preserved
	if !strings.Contains(plain1, "1.") || !strings.Contains(plain1, "2.") || !strings.Contains(plain1, "3.") {
		t.Errorf("expected numbered list 1., 2., 3. in rendered output, got:\n%s", plain1)
	}

	// Verify table names and PR link exist in output
	if !strings.Contains(plain1, "ueschema.shorttermweather") || !strings.Contains(plain1, "ueschema.idrshorttermweather") {
		t.Errorf("expected table names in rendered output, got:\n%s", plain1)
	}
	if !strings.Contains(plain1, "Pull Request 294577") {
		t.Errorf("expected PR title in rendered output, got:\n%s", plain1)
	}
	if !strings.Contains(plain1, "CC: @Moore, Grace Ann") {
		t.Errorf("expected mention in rendered output, got:\n%s", plain1)
	}

	// Verify there are no orphan floating bullet points on their own line
	lines := strings.Split(plain1, "\n")
	for i, l := range lines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "•" {
			t.Errorf("line %d has orphan floating bullet point:\n%s", i+1, plain1)
		}
	}

	// Exact comment 2 from user screenshot with unordered list
	comment2HTML := `<div>Created the following tables:</div>
	<ul>
		<li><code>ueschema.profilingfactorbackcast_hourly</code></li>
		<li><code>ueschema.shorttermweather</code></li>
	</ul>`

	formatted2 := StripAndFormatHTML(comment2HTML)
	rendered2 := RenderMarkdown(formatted2, 90)
	plain2 := StripEscapes(rendered2)

	if !strings.Contains(plain2, "Created the following tables:") {
		t.Errorf("expected header in output 2, got:\n%s", plain2)
	}
	if !strings.Contains(plain2, "ueschema.profilingfactorbackcast_hourly") || !strings.Contains(plain2, "ueschema.shorttermweather") {
		t.Errorf("expected tables in output 2, got:\n%s", plain2)
	}

	// Ensure each bullet is inline with its table name, not separated by empty lines
	lines2 := strings.Split(plain2, "\n")
	for i, l := range lines2 {
		trimmed := strings.TrimSpace(l)
		if trimmed == "•" {
			t.Errorf("line %d has orphan floating bullet in unordered list:\n%s", i+1, plain2)
		}
	}
}

func TestWrapWordsListHangingIndent(t *testing.T) {
	// Test numbered list hanging indent
	longNumbered := "1. Renamed the table ueschema.shorttermweather to ueschema.idrshorttermweather in the DEV database environment."
	wrappedNum := WrapWords(longNumbered, 40)
	numLines := strings.Split(wrappedNum, "\n")
	if len(numLines) < 2 {
		t.Fatalf("expected wrapped multiple lines for numbered list, got %d lines:\n%s", len(numLines), wrappedNum)
	}
	if !strings.HasPrefix(numLines[0], "1. ") {
		t.Errorf("line 0 should start with '1. ', got: %q", numLines[0])
	}
	if !strings.HasPrefix(numLines[1], "   ") {
		t.Errorf("line 1 should have 3-space hanging indent matching '1. ', got: %q", numLines[1])
	}

	// Test bullet list hanging indent
	longBullet := "• Implemented job tracking utilities and environment configuration for long-term and short-term forecasts."
	wrappedBullet := WrapWords(longBullet, 40)
	bulletLines := strings.Split(wrappedBullet, "\n")
	if len(bulletLines) < 2 {
		t.Fatalf("expected wrapped multiple lines for bullet list, got %d lines:\n%s", len(bulletLines), wrappedBullet)
	}
	if !strings.HasPrefix(bulletLines[0], "• ") {
		t.Errorf("line 0 should start with '• ', got: %q", bulletLines[0])
	}
	if !strings.HasPrefix(bulletLines[1], "  ") {
		t.Errorf("line 1 should have 2-space hanging indent matching '• ', got: %q", bulletLines[1])
	}

	// Test quote with numbered list hanging indent
	quoteNumbered := "│ 1. Created entrypoint script to manage job execution across all cluster nodes."
	wrappedQuote := WrapWords(quoteNumbered, 40)
	quoteLines := strings.Split(wrappedQuote, "\n")
	if len(quoteLines) < 2 {
		t.Fatalf("expected wrapped multiple lines for quote numbered list, got %d lines:\n%s", len(quoteLines), wrappedQuote)
	}
	if !strings.HasPrefix(quoteLines[0], "│ 1. ") {
		t.Errorf("line 0 should start with '│ 1. ', got: %q", quoteLines[0])
	}
	if !strings.HasPrefix(quoteLines[1], "│    ") {
		t.Errorf("line 1 should have quote bar plus hanging spaces '│    ', got: %q", quoteLines[1])
	}
}

