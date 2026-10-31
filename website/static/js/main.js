// ado-board-cli interactive frontend scripts
document.addEventListener("DOMContentLoaded", () => {
  const repo = "jkotra/ado-board-cli";
  const releaseBase = `https://github.com/${repo}/releases/latest/download`;

  // 1. Detect User OS
  const detectOS = () => {
    const ua = window.navigator.userAgent.toLowerCase();
    const platform = window.navigator.platform ? window.navigator.platform.toLowerCase() : "";

    if (platform.includes("mac") || ua.includes("macintosh") || ua.includes("mac os")) {
      return "macos";
    }
    if (platform.includes("win") || ua.includes("windows")) {
      return "windows";
    }
    if (platform.includes("linux") || ua.includes("linux") || ua.includes("x11")) {
      return "linux";
    }
    return "unknown";
  };

  const userOS = detectOS();

  // Highlight detected OS download card
  if (userOS !== "unknown") {
    const detectedCard = document.querySelector(`.download-card[data-os="${userOS}"]`);
    if (detectedCard) {
      detectedCard.classList.add("highlighted");
    }

    const primaryBtn = document.getElementById("primary-download-btn");
    const primaryLabel = document.getElementById("primary-download-label");

    if (primaryBtn && primaryLabel) {
      if (userOS === "linux") {
        primaryLabel.textContent = "Download for Linux (x86_64)";
        primaryBtn.href = `${releaseBase}/ado-board-cli_linux_amd64.tar.gz`;
      } else if (userOS === "macos") {
        primaryLabel.textContent = "Download for macOS";
        primaryBtn.href = "#downloads";
      } else if (userOS === "windows") {
        primaryLabel.textContent = "Download for Windows (x86_64)";
        primaryBtn.href = `${releaseBase}/ado-board-cli_windows_amd64.zip`;
      }
    }
  }

  // 2. Fetch Latest Release Tag from GitHub API & update download links
  const updateDownloadLinks = async () => {
    try {
      const response = await fetch(`https://api.github.com/repos/${repo}/releases/latest`);
      if (!response.ok) return;
      const data = await response.json();
      const tagName = data.tag_name; // e.g. "v1.0.0"

      if (tagName) {
        // Update version badge if present
        const badge = document.querySelector(".badge-text");
        if (badge) {
          badge.textContent = `${tagName} — Terminal UI & MCP Copilot`;
        }

        // Match assets with download buttons
        if (Array.isArray(data.assets)) {
          const links = document.querySelectorAll(".download-link");
          links.forEach((link) => {
            const filenamePattern = link.getAttribute("data-filename");
            if (filenamePattern) {
              // Find matching asset in release
              const matchedAsset = data.assets.find(
                (a) => a.name.includes(filenamePattern.replace("ado-board-cli_", "")) || a.name === filenamePattern
              );
              if (matchedAsset) {
                link.href = matchedAsset.browser_download_url;
              } else {
                link.href = `https://github.com/${repo}/releases/download/${tagName}/${filenamePattern}`;
              }
            }
          });
        }
      }
    } catch (e) {
      // Fallback silently if offline or rate-limited
      console.debug("Could not fetch latest release info from GitHub API:", e);
    }
  };

  updateDownloadLinks();

  // 3. Tab switching for Quick Install snippets
  const snippetTabs = document.querySelectorAll(".snippet-tab");
  snippetTabs.forEach((tab) => {
    tab.addEventListener("click", () => {
      const targetTab = tab.getAttribute("data-tab");
      document.querySelectorAll(".snippet-tab").forEach((t) => t.classList.remove("active"));
      document.querySelectorAll(".snippet-panel").forEach((p) => p.classList.remove("active"));

      tab.classList.add("active");
      const targetPanel = document.getElementById(`tab-${targetTab}`);
      if (targetPanel) {
        targetPanel.classList.add("active");
      }
    });
  });

  // 4. Copy to clipboard buttons
  const copyButtons = document.querySelectorAll(".copy-btn");
  copyButtons.forEach((btn) => {
    btn.addEventListener("click", async () => {
      const textToCopy = btn.getAttribute("data-clipboard");
      if (!textToCopy) return;

      try {
        await navigator.clipboard.writeText(textToCopy);
        const originalHtml = btn.innerHTML;
        btn.innerHTML = '<i class="fa-solid fa-check"></i> Copied!';
        btn.classList.add("copied");
        setTimeout(() => {
          btn.innerHTML = originalHtml;
          btn.classList.remove("copied");
        }, 2000);
      } catch (err) {
        console.error("Clipboard copy failed:", err);
      }
    });
  });
});
