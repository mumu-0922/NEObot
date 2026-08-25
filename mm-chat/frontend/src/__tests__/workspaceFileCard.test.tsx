import { renderToStaticMarkup } from "react-dom/server";
import { NextIntlClientProvider } from "next-intl";
import { describe, expect, it } from "vitest";

import WorkspaceFileCard from "../components/content/WorkspaceFileCard";
import contentMessages from "../i18n/locales/en/Content.json";
import messageMessages from "../i18n/locales/en/Message.json";

describe("Workspace File Card", () => {
  it("renders open as the primary action and download as secondary", () => {
    const html = renderToStaticMarkup(
      <NextIntlClientProvider
        locale="en"
        messages={{ Content: contentMessages, Message: messageMessages }}
        timeZone="UTC"
      >
        <WorkspaceFileCard
          file={{
            id: "workspace-file-1",
            type: "workspace_file",
            workspaceId: "0198ca9a-81c6-7c8d-9444-b16da02de9b4",
            path: "reports/result.xlsx",
            fileName: "result.xlsx",
            mimeType:
              "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
            size: 4096,
            version: `sha256:${"a".repeat(64)}`,
          }}
        />
      </NextIntlClientProvider>,
    );

    expect(html).toContain("Open file");
    expect(html).toContain("reports/result.xlsx");
    expect(html).toContain('aria-label="Open workspace file result.xlsx"');
    expect(html).toContain('aria-label="Download result.xlsx"');
  });
});
