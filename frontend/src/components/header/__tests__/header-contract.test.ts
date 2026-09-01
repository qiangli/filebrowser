import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const header = readFileSync(resolve(__dirname, "../HeaderBar.vue"), "utf8");
const listing = readFileSync(
  resolve(__dirname, "../../../views/files/FileListing.vue"),
  "utf8"
);
const files = ["Editor.vue", "Preview.vue"].map((name) =>
  readFileSync(resolve(__dirname, `../../../views/files/${name}`), "utf8")
);

describe("files header contract", () => {
  it("keeps the Apps return control rightmost", () => {
    expect(header).toContain('v-if="showApps"');
    expect(header).toContain('class="action apps-button"');
    // It must carry the CONSOLE's all-apps mark — four rounded squares — not a
    // material-icons glyph, so Files looks like every other app's return
    // control. This SPA is mounted rather than chrome-injected, so it never
    // receives #all-apps-btn and has to draw the same mark itself.
    expect(header).toContain('rx="1.5"');
    expect((header.match(/<rect /g) || []).length).toBe(4);
    expect(header).not.toContain('material-icons">apps<');
    // And it must leave THIS app. A bare href="../" resolved against a route
    // like /files/files/<path> lands back on the files app, which is the bug.
    expect(header).not.toContain('href="../"');
    expect(header).toContain(":href=\"appsHref\"");
    expect(header).toContain("baseURL");
    expect(header.lastIndexOf("apps-button")).toBeGreaterThan(
      header.lastIndexOf('id="more"')
    );
    expect(listing).toContain("<header-bar showMenu showLogo showApps>");
    for (const view of files) expect(view).toContain("showApps");
  });

  it("does not reuse the Apps logo for the view switch", () => {
    const viewIcons = listing.slice(
      listing.indexOf("const viewIcon"),
      listing.indexOf("const headerButtons")
    );
    expect(viewIcons).not.toContain('"grid_view"');
    expect(viewIcons).not.toContain('"apps"');
  });
});
