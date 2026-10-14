#!/usr/bin/env node
// SPDX-FileCopyrightText: The Gommitlint Authors
// SPDX-License-Identifier: CC0-1.0
//
// Zero-dependency WCAG 2.1 contrast verifier for web/assets/style.css.
// Parses the default :root token block plus the @media (prefers-color-scheme: dark)
// override, then asserts each foreground/background pair meets its required ratio.
// Exits non-zero on any failure so CI can gate merges.

import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

const here = dirname(fileURLToPath(import.meta.url));
const cssPath = resolve(here, "../assets/style.css");
const css = readFileSync(cssPath, "utf8");

const PAIRS = [
  { fg: "--color-text",           bg: "--color-bg",      min: 4.5, label: "body text" },
  { fg: "--color-muted",          bg: "--color-bg",      min: 4.5, label: "muted text" },
  { fg: "--color-primary",        bg: "--color-bg",      min: 4.5, label: "primary on bg" },
  { fg: "--color-primary-hover",  bg: "--color-bg",      min: 4.5, label: "primary-hover on bg" },
  { fg: "--color-accent",         bg: "--color-bg",      min: 4.5, label: "accent on bg" },
  { fg: "--color-link",           bg: "--color-bg",      min: 4.5, label: "link on bg" },
  { fg: "--color-focus",          bg: "--color-bg",      min: 3.0, label: "focus ring (non-text)" },
  { fg: "--color-code-text",      bg: "--color-code-bg", min: 4.5, label: "code text" },
  { fg: "--color-btn-primary-text", bg: "--color-primary", min: 4.5, label: "btn-primary text" },
];

// ── parsing ─────────────────────────────────────────────────────────────
const stripComments = (s) => s.replace(/\/\*[\s\S]*?\*\//g, "");

function rootBlock(src) {
  const m = src.match(/:root\s*\{([\s\S]*?)\}/);
  return m ? m[1] : "";
}

function overrideBlock(src) {
  // Matches either the `dark` override (for light-default sites) or the `light`
  // override (for dark-default sites). Returns the :root body + the scheme name.
  const m = src.match(/@media\s*\(\s*prefers-color-scheme\s*:\s*(light|dark)\s*\)\s*\{[\s\S]*?:root\s*\{([\s\S]*?)\}/);
  return m ? { scheme: m[1], body: m[2] } : null;
}

function parseTokens(block, schemeLabel) {
  const tokens = {};
  for (const line of stripComments(block).split("\n")) {
    const m = line.match(/(--[a-z0-9-]+)\s*:\s*([^;]+);/);
    if (!m) continue;
    const name = m[1];
    const value = m[2].trim();
    if (/^#[0-9a-fA-F]{3,8}$/.test(value)) {
      tokens[name] = value;
    } else if (name.startsWith("--color-")) {
      // Non-hex colour tokens (rgb(), hsl(), var(), named colours, gradients)
      // are ignored by the contrast math. Surface them so a skipped check
      // doesn't silently hide a regression.
      console.warn(`  WARN  [${schemeLabel}] ${name}: non-hex value "${value}" — not checked`);
    }
  }
  return tokens;
}

// ── colour math ─────────────────────────────────────────────────────────
function hexToRgb(hex) {
  let h = hex.replace("#", "");
  if (h.length === 3) h = [...h].map((c) => c + c).join("");
  if (h.length !== 6) throw new Error(`bad hex: ${hex}`);
  return [parseInt(h.slice(0, 2), 16), parseInt(h.slice(2, 4), 16), parseInt(h.slice(4, 6), 16)];
}
function channel(c) {
  const s = c / 255;
  return s <= 0.03928 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4);
}
function luminance([r, g, b]) {
  return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b);
}
function contrast(a, b) {
  const la = luminance(hexToRgb(a));
  const lb = luminance(hexToRgb(b));
  const [hi, lo] = la > lb ? [la, lb] : [lb, la];
  return (hi + 0.05) / (lo + 0.05);
}

// ── run ─────────────────────────────────────────────────────────────────
const defaults = parseTokens(rootBlock(css), "default");
const override = overrideBlock(css);
const altScheme = override
  ? { ...defaults, ...parseTokens(override.body, `prefers-${override.scheme}`) }
  : null;

const schemes = [{ name: "default", t: defaults }];
if (altScheme) schemes.push({ name: `prefers-${override.scheme}`, t: altScheme });

let failed = 0;
for (const scheme of schemes) {
  console.log(`\n[${scheme.name}]`);
  for (const p of PAIRS) {
    const fg = scheme.t[p.fg];
    const bg = scheme.t[p.bg];
    if (!fg || !bg) {
      console.log(`  SKIP  ${p.label}: missing token (${p.fg} or ${p.bg})`);
      continue;
    }
    const ratio = contrast(fg, bg);
    const ok = ratio >= p.min;
    if (!ok) failed++;
    console.log(`  ${ok ? "PASS" : "FAIL"}  ${p.label.padEnd(26)} ${ratio.toFixed(2)}:1  (need ${p.min}:1)  ${fg} on ${bg}`);
  }
}

if (failed > 0) {
  console.error(`\n${failed} contrast check(s) failed.`);
  process.exit(1);
}
console.log("\nAll contrast checks passed.");
