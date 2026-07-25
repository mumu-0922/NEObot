import { afterEach, describe, expect, it, vi } from "vitest";
import { triggerBlobDownload } from "../lib/utils/blobDownload";

describe("triggerBlobDownload", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("clicks a hidden download anchor and releases its object URL", () => {
    const click = vi.fn();
    const remove = vi.fn();
    const anchor = { click, remove, href: "", download: "", hidden: false };
    const appendChild = vi.fn();
    vi.stubGlobal("document", {
      createElement: vi.fn(() => anchor),
      body: { appendChild },
    });
    const createObjectURL = vi
      .spyOn(URL, "createObjectURL")
      .mockReturnValue("blob:knowledge-document");
    const revokeObjectURL = vi
      .spyOn(URL, "revokeObjectURL")
      .mockImplementation(() => undefined);
    const blob = new Blob(["source"]);

    triggerBlobDownload(blob, "source.pdf");

    expect(createObjectURL).toHaveBeenCalledWith(blob);
    expect(anchor).toMatchObject({
      href: "blob:knowledge-document",
      download: "source.pdf",
      hidden: true,
    });
    expect(appendChild).toHaveBeenCalledWith(anchor);
    expect(click).toHaveBeenCalledOnce();
    expect(remove).toHaveBeenCalledOnce();
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:knowledge-document");
  });

  it("still removes the anchor and revokes the URL when clicking fails", () => {
    const failure = new Error("download blocked");
    const remove = vi.fn();
    const anchor = {
      click: vi.fn(() => {
        throw failure;
      }),
      remove,
      href: "",
      download: "",
      hidden: false,
    };
    vi.stubGlobal("document", {
      createElement: vi.fn(() => anchor),
      body: { appendChild: vi.fn() },
    });
    vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:knowledge-document");
    const revokeObjectURL = vi
      .spyOn(URL, "revokeObjectURL")
      .mockImplementation(() => undefined);

    expect(() =>
      triggerBlobDownload(new Blob(["source"]), "source.pdf"),
    ).toThrow(failure);
    expect(remove).toHaveBeenCalledOnce();
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:knowledge-document");
  });
});
