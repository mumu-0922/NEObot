import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

import manifest from "@/app/manifest";
import { buildWebApplicationJsonLd } from "@/lib/seo";

const frontendFile = (path: string) => resolve(process.cwd(), path);

function readPngSize(path: string) {
  const png = readFileSync(frontendFile(path));

  expect(png.subarray(0, 8)).toEqual(
    Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]),
  );

  return {
    width: png.readUInt32BE(16),
    height: png.readUInt32BE(20),
    colorType: png.readUInt8(25),
  };
}

describe("NeoBot F1-A logo assets", () => {
  it("keeps the public vector master and inline component geometry aligned", () => {
    const svg = readFileSync(frontendFile("public/logo.svg"), "utf8");
    const icons = readFileSync(
      frontendFile("src/components/ui/Icons.tsx"),
      "utf8",
    );
    expect(svg).toContain('viewBox="0 0 192 192"');
    const ribbonPath =
      "M58 112C37 79 48 35 79 18C99 7 123 15 134 34C148 59 139 91 129 112Q117 118 105 99C113 80 121 57 111 41C104 30 91 28 81 35C60 49 60 77 81 99Q68 118 58 112Z";

    expect(svg.match(new RegExp(ribbonPath, "g"))).toHaveLength(3);
    expect(icons).toContain("React.useId()");
    expect(icons.match(new RegExp(ribbonPath, "g"))).toHaveLength(3);
    expect(svg).toContain('transform="rotate(120 96 96)"');
    expect(svg).toContain('transform="rotate(240 96 96)"');
  });

  it("provides transparent PNG derivatives at the declared PWA sizes", () => {
    expect(readPngSize("public/logo.png")).toEqual({
      width: 192,
      height: 192,
      colorType: 6,
    });
    expect(readPngSize("public/logo-192.png")).toEqual({
      width: 192,
      height: 192,
      colorType: 6,
    });
    expect(readPngSize("public/logo-512.png")).toEqual({
      width: 512,
      height: 512,
      colorType: 6,
    });
  });

  it("publishes matching manifest, SEO, and multi-size favicon metadata", () => {
    const appManifest = manifest();
    const jsonLd = buildWebApplicationJsonLd("en");
    const favicon = readFileSync(frontendFile("src/app/favicon.ico"));

    expect(appManifest.icons).toEqual([
      {
        src: "/logo-192.png",
        sizes: "192x192",
        type: "image/png",
        purpose: "any",
      },
      {
        src: "/logo-512.png",
        sizes: "512x512",
        type: "image/png",
        purpose: "any",
      },
    ]);
    expect(jsonLd.logo).toBe("http://localhost:3000/logo-512.png");
    expect(favicon.readUInt16LE(0)).toBe(0);
    expect(favicon.readUInt16LE(2)).toBe(1);
    expect(favicon.readUInt16LE(4)).toBe(4);
    expect(
      [0, 1, 2, 3].map((index) => favicon.readUInt8(6 + index * 16)),
    ).toEqual([16, 32, 48, 64]);
  });
});
