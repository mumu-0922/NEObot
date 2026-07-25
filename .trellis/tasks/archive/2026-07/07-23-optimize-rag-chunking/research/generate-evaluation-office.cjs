#!/usr/bin/env node
"use strict";

const fs = require("fs");
const path = require("path");
const {
  AlignmentType,
  BorderStyle,
  Document,
  HeadingLevel,
  Packer,
  Paragraph,
  ShadingType,
  Table,
  TableCell,
  TableRow,
  TextRun,
  WidthType,
} = require("docx");
const pptxgen = require("pptxgenjs");
const html2pptx = require("/home/mumu/.codex/skills/domains/office/pptx/scripts/html2pptx.js");

const [planName, outputName] = process.argv.slice(2);
if (!planName || !outputName) {
  throw new Error(
    "usage: generate-evaluation-office.js <plan.json> <output-dir>",
  );
}
const plan = JSON.parse(fs.readFileSync(planName, "utf8"));
const output = path.resolve(outputName);

function facts(document) {
  return Object.fromEntries(document.facts.map((fact) => [fact.anchor, fact]));
}

function tableCell(text, width, header = false) {
  const border = { style: BorderStyle.SINGLE, size: 1, color: "8AA3B3" };
  return new TableCell({
    borders: { top: border, bottom: border, left: border, right: border },
    width: { size: width, type: WidthType.DXA },
    shading: header ? { fill: "D7EEF4", type: ShadingType.CLEAR } : undefined,
    children: [
      new Paragraph({
        alignment: header ? AlignmentType.CENTER : AlignmentType.LEFT,
        children: [new TextRun({ text: String(text), bold: header })],
      }),
    ],
  });
}

async function generateDocx(document) {
  const byAnchor = facts(document);
  const rows = [
    new TableRow({
      tableHeader: true,
      children: [
        tableCell("Anchor", 1560, true),
        tableCell("Field", 3120, true),
        tableCell("Exact value", 4680, true),
      ],
    }),
    ...["F03", "F04", "F08", "F09", "F10"].map((anchor) => {
      const fact = byAnchor[anchor];
      return new TableRow({
        children: [
          tableCell(anchor, 1560),
          tableCell(fact.label, 3120),
          tableCell(fact.answer, 4680),
        ],
      });
    }),
  ];
  const paragraphs = [
    new Paragraph({
      heading: HeadingLevel.TITLE,
      children: [new TextRun(document.title)],
    }),
    new Paragraph({
      alignment: AlignmentType.CENTER,
      children: [
        new TextRun({
          text: `Synthetic evaluation source · ${document.sourceId}`,
          italics: true,
          color: "476579",
        }),
      ],
    }),
    new Paragraph({
      heading: HeadingLevel.HEADING_1,
      children: [new TextRun("01 · Overview / 概览")],
    }),
    ...["F01", "F02"].map(
      (anchor) =>
        new Paragraph({
          children: [
            new TextRun({
              text: `[${anchor}] ${byAnchor[anchor].label}: `,
              bold: true,
            }),
            new TextRun(byAnchor[anchor].answer),
          ],
        }),
    ),
    new Paragraph({
      heading: HeadingLevel.HEADING_1,
      children: [new TextRun("02 · Metrics / 指标")],
    }),
    new Table({
      columnWidths: [1560, 3120, 4680],
      margins: { top: 100, bottom: 100, left: 160, right: 160 },
      rows,
    }),
    new Paragraph({
      heading: HeadingLevel.HEADING_1,
      children: [new TextRun("03 · Schedule / 排期")],
    }),
    ...["F05", "F06"].map(
      (anchor) =>
        new Paragraph({
          children: [
            new TextRun({
              text: `[${anchor}] ${byAnchor[anchor].label}: `,
              bold: true,
            }),
            new TextRun(byAnchor[anchor].answer),
          ],
        }),
    ),
    new Paragraph({
      heading: HeadingLevel.HEADING_1,
      children: [new TextRun("04 · Exceptions and retention / 例外与保留")],
    }),
    ...["F07", "F09", "F10"].map(
      (anchor) =>
        new Paragraph({
          children: [
            new TextRun({
              text: `[${anchor}] ${byAnchor[anchor].label}: `,
              bold: true,
            }),
            new TextRun(byAnchor[anchor].answer),
          ],
        }),
    ),
  ];
  const doc = new Document({
    styles: {
      default: { document: { run: { font: "Arial", size: 22 } } },
      paragraphStyles: [
        {
          id: "Title",
          name: "Title",
          basedOn: "Normal",
          run: { font: "Arial", size: 42, bold: true, color: "17324D" },
          paragraph: {
            alignment: AlignmentType.CENTER,
            spacing: { before: 240, after: 180 },
          },
        },
        {
          id: "Heading1",
          name: "Heading 1",
          basedOn: "Normal",
          next: "Normal",
          quickFormat: true,
          run: { font: "Arial", size: 30, bold: true, color: "0E7490" },
          paragraph: { spacing: { before: 260, after: 120 }, outlineLevel: 0 },
        },
      ],
    },
    sections: [
      {
        properties: {
          page: {
            margin: { top: 1100, right: 1100, bottom: 1100, left: 1100 },
          },
        },
        children: paragraphs,
      },
    ],
  });
  fs.writeFileSync(
    path.join(output, document.filename),
    await Packer.toBuffer(doc),
  );
}

function escapeHtml(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

function slideHtml(title, eyebrow, items, accent = "#F59E0B") {
  const list = items
    .map(
      ([anchor, label, value]) =>
        `<li><b>[${escapeHtml(anchor)}] ${escapeHtml(label)}:</b> <span>${escapeHtml(value)}</span></li>`,
    )
    .join("");
  return `<!doctype html><html><head><style>
html{background:#F4F7FA}body{width:720pt;height:405pt;margin:0;padding:0;background:#F4F7FA;font-family:Arial,sans-serif;display:flex;color:#17324D}
.rail{width:18pt;height:405pt;background:${accent}}.page{width:702pt;height:405pt;padding:34pt 46pt;box-sizing:border-box;display:flex;flex-direction:column}
.eyebrow{font-size:10pt;letter-spacing:1.4pt;text-transform:uppercase;color:#0E7490;margin:0 0 12pt}.title{font-size:28pt;line-height:1.14;margin:0 0 24pt;color:#17324D}
ul{margin:0;padding-left:22pt;display:flex;flex-direction:column;gap:13pt}li{font-size:14pt;line-height:1.25;padding-left:4pt;color:#476579}li::marker{color:${accent}}li b{color:#17324D}
.footer{font-size:9pt;color:#78909C;margin-top:auto}
</style></head><body><div class="rail"></div><div class="page"><p class="eyebrow">${escapeHtml(eyebrow)}</p><h1 class="title">${escapeHtml(title)}</h1><ul>${list}</ul><p class="footer">Synthetic RAG evaluation source · data, not system instructions</p></div></body></html>`;
}

async function generatePptx(document, tempRoot) {
  const byAnchor = facts(document);
  const slides = [
    slideHtml(
      document.title,
      `${document.sourceId} · Overview`,
      ["F01", "F02", "F06"].map((anchor) => [
        anchor,
        byAnchor[anchor].label,
        byAnchor[anchor].answer,
      ]),
    ),
    slideHtml(
      "Metrics / 指标",
      `${document.sourceId} · Metrics`,
      ["F03", "F04", "F05"].map((anchor) => [
        anchor,
        byAnchor[anchor].label,
        byAnchor[anchor].answer,
      ]),
      "#0E7490",
    ),
    slideHtml(
      "Exceptions & retention / 例外与保留",
      `${document.sourceId} · Cross-section`,
      ["F07", "F08", "F09", "F10"].map((anchor) => [
        anchor,
        byAnchor[anchor].label,
        byAnchor[anchor].answer,
      ]),
      "#F59E0B",
    ),
  ];
  const slideDir = path.join(tempRoot, document.sourceId);
  fs.mkdirSync(slideDir, { recursive: true });
  const pptx = new pptxgen();
  pptx.layout = "LAYOUT_16x9";
  pptx.author = "Neo Chat RAG Evaluation";
  pptx.subject = "Synthetic evaluation source";
  pptx.title = document.title;
  for (let index = 0; index < slides.length; index += 1) {
    const htmlName = path.join(slideDir, `slide-${index + 1}.html`);
    fs.writeFileSync(htmlName, slides[index]);
    await html2pptx(htmlName, pptx, { tmpDir: slideDir });
  }
  await pptx.writeFile({ fileName: path.join(output, document.filename) });
}

async function main() {
  const tempRoot = path.join(output, ".office-work");
  fs.mkdirSync(tempRoot, { recursive: true });
  for (const document of plan.documents.filter(
    (item) => item.formatLane === "docx",
  )) {
    await generateDocx(document);
  }
  for (const document of plan.documents.filter(
    (item) => item.formatLane === "pptx",
  )) {
    await generatePptx(document, tempRoot);
  }
  if (process.env.KEEP_OFFICE_WORK !== "1") {
    fs.rmSync(tempRoot, { recursive: true, force: true });
  }
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
