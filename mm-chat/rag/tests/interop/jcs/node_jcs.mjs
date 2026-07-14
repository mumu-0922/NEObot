#!/usr/bin/env node
// Independent Node 22 JCS fixture implementation; standard library only.

import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const MAX_SAFE_INTEGER = 9007199254740991;
const MANIFESTS = ["c1-contract-profile-v1.json", "rfc8785-v1.json"];
const LOGICAL_MANIFEST = "logical-hash-golden-v1.json";
const LOGICAL_SOURCE_PATH = "../parser_contracts/logical_hash/golden-v1.json";
const LOGICAL_CASE_COUNT = 24;
const LOGICAL_FRAMING =
  "ASCII(domain-with-one-terminal-LF) || RFC8785(envelopeWithoutDomain)";
const HEX_PATTERN = /^(?:[0-9a-f]{2})*$/;
const SHA_PATTERN = /^[0-9a-f]{64}$/;
const BOMS = [
  Buffer.from("0000feff", "hex"),
  Buffer.from("fffe0000", "hex"),
  Buffer.from("efbbbf", "hex"),
  Buffer.from("feff", "hex"),
  Buffer.from("fffe", "hex"),
];

class ConformanceError extends Error {
  constructor(code, caseId = "") {
    super(code);
    this.code = code;
    this.caseId = caseId;
  }
}

function sha256(value) {
  return createHash("sha256").update(value).digest("hex");
}

class StrictJsonParser {
  constructor(raw, profile) {
    if (BOMS.some((bom) => raw.subarray(0, bom.length).equals(bom))) {
      throw new ConformanceError("BOM_FORBIDDEN");
    }
    if (raw.includes(0)) {
      throw new ConformanceError("NUL_FORBIDDEN");
    }
    try {
      this.text = new TextDecoder("utf-8", { fatal: true }).decode(raw);
    } catch (error) {
      throw new ConformanceError("JSON_INVALID");
    }
    this.index = 0;
    this.profile = profile;
    this.rejectNul = profile === "c1-contract-profile";
  }

  parse() {
    const value = this.parseValue();
    this.skipWhitespace();
    if (this.index !== this.text.length) {
      throw new ConformanceError("JSON_INVALID");
    }
    return value;
  }

  skipWhitespace() {
    while (this.index < this.text.length && " \t\r\n".includes(this.text[this.index])) {
      this.index += 1;
    }
  }

  parseValue() {
    this.skipWhitespace();
    const character = this.text[this.index];
    if (character === '"') return this.parseString();
    if (character === "[") return this.parseArray();
    if (character === "{") return this.parseObject();
    if (character === "t") return this.parseLiteral("true", true);
    if (character === "f") return this.parseLiteral("false", false);
    if (character === "n") return this.parseLiteral("null", null);
    if (character === "-" || (character >= "0" && character <= "9")) {
      return this.parseNumber();
    }
    throw new ConformanceError("JSON_INVALID");
  }

  parseLiteral(token, value) {
    if (this.text.slice(this.index, this.index + token.length) !== token) {
      throw new ConformanceError("JSON_INVALID");
    }
    this.index += token.length;
    return value;
  }

  parseArray() {
    this.index += 1;
    const result = [];
    this.skipWhitespace();
    if (this.text[this.index] === "]") {
      this.index += 1;
      return result;
    }
    while (true) {
      result.push(this.parseValue());
      this.skipWhitespace();
      const separator = this.text[this.index];
      this.index += 1;
      if (separator === "]") return result;
      if (separator !== ",") throw new ConformanceError("JSON_INVALID");
    }
  }

  parseObject() {
    this.index += 1;
    const result = Object.create(null);
    const keys = new Set();
    this.skipWhitespace();
    if (this.text[this.index] === "}") {
      this.index += 1;
      return result;
    }
    while (true) {
      this.skipWhitespace();
      if (this.text[this.index] !== '"') throw new ConformanceError("JSON_INVALID");
      const key = this.parseString();
      if (keys.has(key)) throw new ConformanceError("DUPLICATE_KEY");
      keys.add(key);
      this.skipWhitespace();
      if (this.text[this.index] !== ":") throw new ConformanceError("JSON_INVALID");
      this.index += 1;
      result[key] = this.parseValue();
      this.skipWhitespace();
      const separator = this.text[this.index];
      this.index += 1;
      if (separator === "}") return result;
      if (separator !== ",") throw new ConformanceError("JSON_INVALID");
    }
  }

  parseString() {
    this.index += 1;
    let output = "";
    while (this.index < this.text.length) {
      const character = this.text[this.index];
      const code = this.text.charCodeAt(this.index);
      this.index += 1;
      if (character === '"') return output;
      if (character === "\\") {
        output += this.parseEscape();
        continue;
      }
      if (code < 0x20) throw new ConformanceError("JSON_INVALID");
      if (code === 0 && this.rejectNul) throw new ConformanceError("NUL_FORBIDDEN");
      if (code >= 0xd800 && code <= 0xdbff) {
        if (this.index >= this.text.length) {
          throw new ConformanceError("SURROGATE_FORBIDDEN");
        }
        const low = this.text.charCodeAt(this.index);
        if (low < 0xdc00 || low > 0xdfff) {
          throw new ConformanceError("SURROGATE_FORBIDDEN");
        }
        output += character + this.text[this.index];
        this.index += 1;
      } else if (code >= 0xdc00 && code <= 0xdfff) {
        throw new ConformanceError("SURROGATE_FORBIDDEN");
      } else {
        output += character;
      }
    }
    throw new ConformanceError("JSON_INVALID");
  }

  parseEscape() {
    if (this.index >= this.text.length) throw new ConformanceError("JSON_INVALID");
    const escape = this.text[this.index];
    this.index += 1;
    const simple = {
      '"': '"',
      "\\": "\\",
      "/": "/",
      b: "\b",
      f: "\f",
      n: "\n",
      r: "\r",
      t: "\t",
    };
    if (Object.hasOwn(simple, escape)) return simple[escape];
    if (escape !== "u") throw new ConformanceError("JSON_INVALID");
    const high = this.parseHexCodeUnit();
    if (high === 0 && this.rejectNul) throw new ConformanceError("NUL_FORBIDDEN");
    if (high >= 0xd800 && high <= 0xdbff) {
      if (this.text.slice(this.index, this.index + 2) !== "\\u") {
        throw new ConformanceError("SURROGATE_FORBIDDEN");
      }
      this.index += 2;
      const low = this.parseHexCodeUnit();
      if (low < 0xdc00 || low > 0xdfff) {
        throw new ConformanceError("SURROGATE_FORBIDDEN");
      }
      return String.fromCodePoint(0x10000 + ((high - 0xd800) << 10) + low - 0xdc00);
    }
    if (high >= 0xdc00 && high <= 0xdfff) {
      throw new ConformanceError("SURROGATE_FORBIDDEN");
    }
    return String.fromCodePoint(high);
  }

  parseHexCodeUnit() {
    const token = this.text.slice(this.index, this.index + 4);
    if (!/^[0-9a-fA-F]{4}$/.test(token)) throw new ConformanceError("JSON_INVALID");
    this.index += 4;
    return Number.parseInt(token, 16);
  }

  parseNumber() {
    const match = /^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?/.exec(
      this.text.slice(this.index),
    );
    if (match === null) throw new ConformanceError("JSON_INVALID");
    const token = match[0];
    this.index += token.length;
    if (this.profile === "c1-contract-profile" && /[.eE]/.test(token)) {
      throw new ConformanceError("FLOAT_FORBIDDEN");
    }
    const value = Number(token);
    if (!Number.isFinite(value)) throw new ConformanceError("NON_FINITE");
    if (this.profile === "c1-contract-profile" && !Number.isSafeInteger(value)) {
      throw new ConformanceError("UNSAFE_INTEGER");
    }
    return value;
  }
}

function parseJson(raw, profile) {
  return new StrictJsonParser(raw, profile).parse();
}

function validateScalarString(value, rejectNul = false) {
  for (let index = 0; index < value.length; index += 1) {
    const code = value.charCodeAt(index);
    if (code === 0 && rejectNul) throw new ConformanceError("NUL_FORBIDDEN");
    if (code >= 0xd800 && code <= 0xdbff) {
      index += 1;
      if (index >= value.length) throw new ConformanceError("SURROGATE_FORBIDDEN");
      const low = value.charCodeAt(index);
      if (low < 0xdc00 || low > 0xdfff) {
        throw new ConformanceError("SURROGATE_FORBIDDEN");
      }
    } else if (code >= 0xdc00 && code <= 0xdfff) {
      throw new ConformanceError("SURROGATE_FORBIDDEN");
    }
  }
}

function canonicalize(value) {
  if (value === null) return Buffer.from("null", "ascii");
  if (value === true) return Buffer.from("true", "ascii");
  if (value === false) return Buffer.from("false", "ascii");
  if (typeof value === "number") {
    if (!Number.isFinite(value)) throw new ConformanceError("NON_FINITE");
    return Buffer.from(JSON.stringify(value), "ascii");
  }
  if (typeof value === "string") {
    validateScalarString(value);
    return Buffer.from(JSON.stringify(value), "utf8");
  }
  if (Array.isArray(value)) {
    const members = value.map((item) => canonicalize(item));
    return Buffer.concat([Buffer.from("["), Buffer.from(members.join(",")), Buffer.from("]")]);
  }
  if (typeof value === "object") {
    const members = Object.keys(value)
      .sort()
      .map((key) => {
        validateScalarString(key);
        return Buffer.concat([
          Buffer.from(JSON.stringify(key), "utf8"),
          Buffer.from(":"),
          canonicalize(value[key]),
        ]);
      });
    const joined = [];
    members.forEach((member, index) => {
      if (index > 0) joined.push(Buffer.from(","));
      joined.push(member);
    });
    return Buffer.concat([Buffer.from("{"), ...joined, Buffer.from("}")]);
  }
  throw new ConformanceError("JSON_INVALID");
}

function expectObject(value, fields) {
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    throw new ConformanceError("MANIFEST_INVALID");
  }
  const actual = Object.keys(value).sort();
  const expected = [...fields].sort();
  if (actual.length !== expected.length || actual.some((item, index) => item !== expected[index])) {
    throw new ConformanceError("MANIFEST_INVALID");
  }
  return value;
}

function expectText(value, asciiOnly = false) {
  if (typeof value !== "string" || value.length === 0) {
    throw new ConformanceError("MANIFEST_INVALID");
  }
  if (asciiOnly && (!/^[\x20-\x7e]+$/.test(value))) {
    throw new ConformanceError("MANIFEST_INVALID");
  }
  return value;
}

function expectSha(value) {
  const text = expectText(value, true);
  if (!SHA_PATTERN.test(text)) throw new ConformanceError("MANIFEST_INVALID");
  return text;
}

function decodeHex(value) {
  if (typeof value !== "string" || !HEX_PATTERN.test(value)) {
    throw new ConformanceError("MANIFEST_INVALID");
  }
  return Buffer.from(value, "hex");
}

function validateCase(value, profile) {
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    throw new ConformanceError("MANIFEST_INVALID");
  }
  const fields = new Set(["caseId", "kind", "expect", "inputSha256"]);
  if (value.kind === "json") fields.add("inputHex");
  else if (value.kind === "ieee754" && profile === "rfc8785") fields.add("ieee754Hex");
  else throw new ConformanceError("MANIFEST_INVALID");
  if (value.expect === "accept") {
    fields.add("expectedHex");
    fields.add("expectedSha256");
  } else if (value.expect === "reject") fields.add("reasonCode");
  else throw new ConformanceError("MANIFEST_INVALID");
  expectObject(value, fields);
  expectText(value.caseId, true);
  expectSha(value.inputSha256);
  const input = decodeHex(value.kind === "json" ? value.inputHex : value.ieee754Hex);
  if (value.kind === "ieee754" && input.length !== 8) {
    throw new ConformanceError("MANIFEST_INVALID");
  }
  if (sha256(input) !== value.inputSha256) {
    throw new ConformanceError("FIXTURE_HASH_MISMATCH");
  }
  if (value.expect === "accept") {
    const expected = decodeHex(value.expectedHex);
    expectSha(value.expectedSha256);
    if (sha256(expected) !== value.expectedSha256) {
      throw new ConformanceError("FIXTURE_HASH_MISMATCH");
    }
  } else expectText(value.reasonCode, true);
  return value;
}

function loadManifest(path) {
  const raw = readFileSync(path);
  const manifest = parseJson(raw, "c1-contract-profile");
  if (!canonicalize(manifest).equals(raw)) {
    throw new ConformanceError("MANIFEST_NOT_CANONICAL");
  }
  expectObject(
    manifest,
    new Set([
      "schemaVersion",
      "suiteId",
      "profile",
      "provenance",
      "fixtureSetSha256",
      "cases",
    ]),
  );
  if (manifest.schemaVersion !== "mm-chat.jcs-vector-manifest.v1") {
    throw new ConformanceError("MANIFEST_INVALID");
  }
  if (!["c1-contract-profile", "rfc8785"].includes(manifest.profile)) {
    throw new ConformanceError("MANIFEST_INVALID");
  }
  expectText(manifest.suiteId, true);
  expectObject(
    manifest.provenance,
    new Set(["source", "sourceUrl", "revision", "materialSha256", "license", "licenseFile"]),
  );
  for (const field of ["source", "sourceUrl", "revision", "license", "licenseFile"]) {
    expectText(manifest.provenance[field]);
  }
  if (!Array.isArray(manifest.cases) || manifest.cases.length === 0) {
    throw new ConformanceError("MANIFEST_INVALID");
  }
  for (const testCase of manifest.cases) validateCase(testCase, manifest.profile);
  const fixtureHash = sha256(canonicalize(manifest.cases));
  if (fixtureHash !== expectSha(manifest.fixtureSetSha256)) {
    throw new ConformanceError("FIXTURE_SET_HASH_MISMATCH");
  }
  if (fixtureHash !== expectSha(manifest.provenance.materialSha256)) {
    throw new ConformanceError("PROVENANCE_HASH_MISMATCH");
  }
  return manifest;
}

function expectLogicalName(value) {
  const name = expectText(value, true);
  if (!/^[\x21-\x7e]+$/.test(name)) {
    throw new ConformanceError("LOGICAL_GOLDEN_INVALID");
  }
  return name;
}

function expectLogicalDomain(value) {
  if (
    typeof value !== "string" ||
    !value.endsWith("\n") ||
    value.indexOf("\n") !== value.length - 1 ||
    !/^[\x21-\x7e]+$/.test(value.slice(0, -1))
  ) {
    throw new ConformanceError("LOGICAL_GOLDEN_INVALID");
  }
  return Buffer.from(value, "ascii");
}

function loadLogicalSuite(fixtures) {
  const manifestRaw = readFileSync(resolve(fixtures, LOGICAL_MANIFEST));
  const manifest = parseJson(manifestRaw, "c1-contract-profile");
  if (!canonicalize(manifest).equals(manifestRaw)) {
    throw new ConformanceError("MANIFEST_NOT_CANONICAL");
  }
  expectObject(
    manifest,
    new Set(["caseCount", "profile", "provenance", "schemaVersion", "suiteId"]),
  );
  if (
    manifest.schemaVersion !== "mm-chat.jcs-logical-hash-manifest.v1" ||
    manifest.suiteId !== "logical-hash-golden-v1" ||
    manifest.profile !== "c1-contract-profile" ||
    !Number.isSafeInteger(manifest.caseCount) ||
    manifest.caseCount !== LOGICAL_CASE_COUNT
  ) {
    throw new ConformanceError("MANIFEST_INVALID");
  }
  expectObject(
    manifest.provenance,
    new Set([
      "license",
      "licenseFile",
      "materialSha256",
      "revision",
      "source",
      "sourcePath",
    ]),
  );
  for (const field of ["license", "licenseFile", "revision", "source"]) {
    expectText(manifest.provenance[field]);
  }
  if (
    manifest.provenance.sourcePath !== LOGICAL_SOURCE_PATH ||
    manifest.provenance.licenseFile !== "README.md"
  ) {
    throw new ConformanceError("MANIFEST_INVALID");
  }

  const sourceRaw = readFileSync(resolve(fixtures, LOGICAL_SOURCE_PATH));
  const sourceHash = sha256(sourceRaw);
  if (sourceHash !== expectSha(manifest.provenance.materialSha256)) {
    throw new ConformanceError("PROVENANCE_HASH_MISMATCH");
  }
  const golden = parseJson(sourceRaw, "c1-contract-profile");
  if (!canonicalize(golden).equals(sourceRaw)) {
    throw new ConformanceError("LOGICAL_GOLDEN_NOT_CANONICAL");
  }
  expectObject(golden, new Set(["algorithm", "framing", "vectors"]));
  if (
    golden.algorithm !== "sha-256" ||
    golden.framing !== LOGICAL_FRAMING ||
    !Array.isArray(golden.vectors) ||
    golden.vectors.length !== LOGICAL_CASE_COUNT
  ) {
    throw new ConformanceError("LOGICAL_GOLDEN_INVALID");
  }

  const names = new Set();
  const results = golden.vectors.map((vector) => {
    expectObject(
      vector,
      new Set(["domain", "envelopeWithoutDomain", "expectedSha256", "name"]),
    );
    const name = expectLogicalName(vector.name);
    if (names.has(name)) throw new ConformanceError("LOGICAL_GOLDEN_INVALID");
    names.add(name);
    const domain = expectLogicalDomain(vector.domain);
    const expected = expectSha(vector.expectedSha256);
    const envelope = vector.envelopeWithoutDomain;
    if (
      envelope === null ||
      typeof envelope !== "object" ||
      Array.isArray(envelope) ||
      Object.hasOwn(envelope, "domain") ||
      envelope.envelopeKind !== name
    ) {
      throw new ConformanceError("LOGICAL_GOLDEN_INVALID");
    }
    const digest = sha256(Buffer.concat([domain, canonicalize(envelope)]));
    if (digest !== expected) {
      throw new ConformanceError("LOGICAL_HASH_MISMATCH", name);
    }
    return { digest, name };
  });
  return { rawSha256: sourceHash, results, suiteId: manifest.suiteId };
}

function executeCase(testCase, profile) {
  try {
    let canonical;
    if (testCase.kind === "json") {
      const raw = decodeHex(testCase.inputHex);
      canonical = canonicalize(parseJson(raw, profile));
      if (profile === "c1-contract-profile" && !canonical.equals(raw)) {
        throw new ConformanceError("NON_CANONICAL");
      }
    } else {
      const raw = decodeHex(testCase.ieee754Hex);
      canonical = canonicalize(raw.readDoubleBE(0));
    }
    if (testCase.expect === "reject") throw new ConformanceError("UNEXPECTED_ACCEPT");
    const expected = decodeHex(testCase.expectedHex);
    if (!canonical.equals(expected)) throw new ConformanceError("CANONICAL_BYTES_MISMATCH");
    const digest = sha256(canonical);
    if (digest !== testCase.expectedSha256) {
      throw new ConformanceError("CANONICAL_HASH_MISMATCH");
    }
    return ["accept", digest];
  } catch (error) {
    if (
      error instanceof ConformanceError &&
      testCase.expect === "reject" &&
      error.code === testCase.reasonCode
    ) {
      return ["reject", error.code];
    }
    if (error instanceof ConformanceError) {
      throw new ConformanceError(error.code, testCase.caseId);
    }
    throw error;
  }
}

function run(fixtures) {
  const transcript = [];
  let caseCount = 0;
  for (const filename of MANIFESTS) {
    const manifest = loadManifest(resolve(fixtures, filename));
    for (const testCase of manifest.cases) {
      const [outcome, digest] = executeCase(testCase, manifest.profile);
      transcript.push(`${manifest.suiteId}\0${testCase.caseId}\0${outcome}\0${digest}\n`);
      caseCount += 1;
    }
  }
  const logical = loadLogicalSuite(fixtures);
  for (const result of logical.results) {
    transcript.push(`${logical.suiteId}\0${result.name}\0accept\0${result.digest}\n`);
    caseCount += 1;
  }
  return {
    caseCount,
    implementation: "node",
    logicalGoldenRawSha256: logical.rawSha256,
    resultSha256: sha256(Buffer.from(transcript.join(""), "ascii")),
    status: "pass",
    suiteCount: MANIFESTS.length + 1,
    version: process.versions.node,
  };
}

function writeSummary(summary) {
  const encoded = Buffer.concat([canonicalize(summary), Buffer.from("\n")]);
  if (!/^[\x00-\x7f]*$/.test(encoded.toString("latin1"))) {
    throw new Error("summary is not ASCII");
  }
  process.stdout.write(encoded);
}

function main() {
  const args = process.argv.slice(2);
  if (args.length !== 2 || args[0] !== "--fixtures") {
    writeSummary({
      error: "CLI_ARGUMENT_INVALID",
      failedCase: "",
      implementation: "node",
      status: "fail",
      version: process.versions.node,
    });
    return 2;
  }
  try {
    writeSummary(run(resolve(args[1])));
    return 0;
  } catch (error) {
    writeSummary({
      error: error instanceof ConformanceError ? error.code : "INTERNAL_ERROR",
      failedCase: error instanceof ConformanceError ? error.caseId : "",
      implementation: "node",
      status: "fail",
      version: process.versions.node,
    });
    return 1;
  }
}

process.exitCode = main();
