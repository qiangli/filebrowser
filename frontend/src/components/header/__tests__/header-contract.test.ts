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
    expect(header).toContain('href="../"');
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
