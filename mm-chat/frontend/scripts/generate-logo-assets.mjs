import { readFile, writeFile } from "node:fs/promises";
import { resolve } from "node:path";

import { chromium } from "@playwright/test";

const frontendRoot = resolve(import.meta.dirname, "..");
const publicRoot = resolve(frontendRoot, "public");
const svg = await readFile(resolve(publicRoot, "logo.svg"), "utf8");
const svgDataUrl = `data:image/svg+xml;base64,${Buffer.from(svg).toString("base64")}`;

const browser = await chromium.launch({ headless: true });

async function renderPng(size) {
  const context = await browser.newContext({
    viewport: { width: size, height: size },
    deviceScaleFactor: 1,
  });
  const page = await context.newPage();

  await page.setContent(`
    <!doctype html>
    <html>
      <head>
        <style>
          html, body, img { width: 100%; height: 100%; margin: 0; padding: 0; }
          html, body { background: transparent; overflow: hidden; }
          img { display: block; }
        </style>
      </head>
      <body><img src="${svgDataUrl}" alt="" /></body>
    </html>
  `);
  await page.locator("img").waitFor({ state: "visible" });

  const png = await page.screenshot({
    type: "png",
    omitBackground: true,
  });
  await context.close();
  return png;
}

function buildIco(images) {
  const directorySize = 6 + images.length * 16;
  const header = Buffer.alloc(directorySize);
  header.writeUInt16LE(0, 0);
  header.writeUInt16LE(1, 2);
  header.writeUInt16LE(images.length, 4);

  let imageOffset = directorySize;
  images.forEach(({ size, png }, index) => {
    const entryOffset = 6 + index * 16;
    header.writeUInt8(size === 256 ? 0 : size, entryOffset);
    header.writeUInt8(size === 256 ? 0 : size, entryOffset + 1);
    header.writeUInt8(0, entryOffset + 2);
    header.writeUInt8(0, entryOffset + 3);
    header.writeUInt16LE(1, entryOffset + 4);
    header.writeUInt16LE(32, entryOffset + 6);
    header.writeUInt32LE(png.length, entryOffset + 8);
    header.writeUInt32LE(imageOffset, entryOffset + 12);
    imageOffset += png.length;
  });

  return Buffer.concat([header, ...images.map(({ png }) => png)]);
}

try {
  const logo192 = await renderPng(192);
  const logo512 = await renderPng(512);
  const faviconImages = await Promise.all(
    [16, 32, 48, 64].map(async (size) => ({
      size,
      png: await renderPng(size),
    })),
  );

  await Promise.all([
    writeFile(resolve(publicRoot, "logo.png"), logo192),
    writeFile(resolve(publicRoot, "logo-192.png"), logo192),
    writeFile(resolve(publicRoot, "logo-512.png"), logo512),
    writeFile(
      resolve(frontendRoot, "src/app/favicon.ico"),
      buildIco(faviconImages),
    ),
  ]);
} finally {
  await browser.close();
}
