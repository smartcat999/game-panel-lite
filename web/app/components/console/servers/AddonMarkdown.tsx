"use client";

import { Fragment, ReactNode, useEffect, useMemo, useState } from "react";

const KNOWN_HTML_TAGS = new Set([
  "a",
  "abbr",
  "article",
  "b",
  "blockquote",
  "br",
  "center",
  "code",
  "del",
  "details",
  "div",
  "em",
  "figcaption",
  "figure",
  "footer",
  "h1",
  "h2",
  "h3",
  "h4",
  "h5",
  "h6",
  "header",
  "hr",
  "i",
  "img",
  "ins",
  "kbd",
  "li",
  "mark",
  "ol",
  "p",
  "picture",
  "pre",
  "s",
  "section",
  "small",
  "source",
  "span",
  "strike",
  "strong",
  "sub",
  "summary",
  "sup",
  "table",
  "tbody",
  "td",
  "th",
  "thead",
  "tr",
  "u",
  "ul",
]);

const DROPPED_TAGS = new Set([
  "script",
  "style",
  "iframe",
  "object",
  "embed",
  "form",
  "input",
  "button",
  "textarea",
  "select",
  "meta",
  "link",
  "base",
  "svg",
  "math",
]);

const HTML_BLOCK_START_RE =
  /^\s*<\/?(?:div|p|table|thead|tbody|tr|th|td|ul|ol|li|details|summary|blockquote|pre|center|h[1-6]|hr|figure|section)\b/i;

function escapeHtml(raw: string): string {
  return raw
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function sanitizeHref(raw: string | null): string | null {
  if (!raw) return null;
  const trimmed = raw.trim();
  if (/^https?:\/\//i.test(trimmed) || /^mailto:/i.test(trimmed)) {
    return trimmed;
  }
  return null;
}

function sanitizeImgSrc(raw: string | null): string | null {
  if (!raw) return null;
  const trimmed = raw.trim();
  if (
    /^https?:\/\//i.test(trimmed) ||
    trimmed.startsWith("/apis/core.gamepanel.io/v1/apps/") ||
    trimmed.startsWith("/apis/registry.gamepanel.io/v1/packages/")
  ) {
    return trimmed;
  }
  return null;
}

function applyInlineMarkdown(line: string, restoreCode: (s: string) => string): string {
  // Escape angle-bracket placeholders like <player> or <amount> that are not HTML tags.
  let s = line.replace(/<(\/?)([a-zA-Z][a-zA-Z0-9_-]*)(\s[^>]*)?>/g, (full, slash, tag) => {
    if (KNOWN_HTML_TAGS.has(String(tag).toLowerCase()) || DROPPED_TAGS.has(String(tag).toLowerCase())) {
      return full;
    }
    return `&lt;${slash}${tag}&gt;`;
  });

  // Markdown images: ![alt](url)
  s = s.replace(
    /!\[([^\]]*)\]\((https?:\/\/[^\s)]+|\/apis\/[^\s)]+)\)/g,
    (_, alt: string, url: string) =>
      `<img src="${escapeHtml(url)}" alt="${escapeHtml(alt)}" />`,
  );

  // Markdown links (including nested badge images): [label](url)
  s = s.replace(
    /\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g,
    (_, label: string, url: string) =>
      `<a href="${escapeHtml(url)}">${label}</a>`,
  );

  // Bold, italic, strikethrough
  s = s.replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>");
  s = s.replace(/~~([^~]+)~~/g, "<del>$1</del>");
  s = s.replace(/(^|[\s(])\*([^*\n]+)\*(?=[\s).,!?:;]|$)/g, "$1<em>$2</em>");

  return restoreCode(s);
}

function isPipeTableSeparator(line: string): boolean {
  const t = line.trim();
  return /^\|?[\s:-]+\|[\s|:-]+\|?$/.test(t) && t.includes("-");
}

function splitPipeRow(line: string): string[] {
  let t = line.trim();
  if (t.startsWith("|")) t = t.slice(1);
  if (t.endsWith("|")) t = t.slice(0, -1);
  return t.split("|").map((c) => c.trim());
}

function compileMarkdownToHtml(raw: string): string {
  const codeBlocks: string[] = [];
  const inlineCodes: string[] = [];

  let text = raw.replace(/\r\n/g, "\n").replace(/\r/g, "\n");

  // 1. Stash fenced code blocks first so inner HTML/Markdown is untouched.
  text = text.replace(/```([a-zA-Z0-9_-]*)\n([\s\S]*?)```/g, (_, _lang: string, body: string) => {
    const idx = codeBlocks.length;
    codeBlocks.push(`<pre><code>${escapeHtml(body.replace(/\n$/, ""))}</code></pre>`);
    return `\n\u0000CODEBLOCK_${idx}\u0000\n`;
  });

  // 2. Stash inline code spans so `<player>` inside backticks is literal.
  text = text.replace(/`([^`\n]+)`/g, (_, code: string) => {
    const idx = inlineCodes.length;
    inlineCodes.push(`<code>${escapeHtml(code)}</code>`);
    return `\u0000INLINECODE_${idx}\u0000`;
  });

  const restoreInline = (input: string) =>
    input.replace(/\u0000INLINECODE_(\d+)\u0000/g, (_, n: string) => inlineCodes[Number(n)] || "");

  const lines = text.split("\n");
  const out: string[] = [];
  let paraBuf: string[] = [];
  let rawPDepth = 0;

  const flushPara = () => {
    if (paraBuf.length === 0) return;
    const renderedLines = paraBuf.map((l) => applyInlineMarkdown(l, restoreInline));
    const hasInlineHtml = paraBuf.some((l) => /<[a-zA-Z/]|&nbsp;/i.test(l));
    const joined = renderedLines.join(hasInlineHtml ? " " : "<br />");
    out.push(rawPDepth > 0 ? joined : `<p>${joined}</p>`);
    paraBuf = [];
  };

  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    const trimmed = line.trim();

    if (!trimmed) {
      flushPara();
      i++;
      continue;
    }

    const codeMatch = /^\u0000CODEBLOCK_(\d+)\u0000$/.exec(trimmed);
    if (codeMatch) {
      flushPara();
      out.push(codeBlocks[Number(codeMatch[1])] || "");
      i++;
      continue;
    }

    const headingMatch = /^(#{1,6})\s+(.+?)\s*#*$/.exec(trimmed);
    if (headingMatch) {
      flushPara();
      const level = headingMatch[1].length;
      out.push(`<h${level}>${applyInlineMarkdown(headingMatch[2], restoreInline)}</h${level}>`);
      i++;
      continue;
    }

    if (/^(?:---+|___+|\*\*\*+)$/.test(trimmed)) {
      flushPara();
      out.push("<hr />");
      i++;
      continue;
    }

    // Pipe table
    if (trimmed.includes("|") && i + 1 < lines.length && isPipeTableSeparator(lines[i + 1])) {
      flushPara();
      const headers = splitPipeRow(line);
      i += 2;
      const bodyRows: string[][] = [];
      while (i < lines.length && lines[i].trim().includes("|") && lines[i].trim() !== "") {
        bodyRows.push(splitPipeRow(lines[i]));
        i++;
      }
      const headHtml = `<thead><tr>${headers
        .map((h) => `<th>${applyInlineMarkdown(h, restoreInline)}</th>`)
        .join("")}</tr></thead>`;
      const bodyHtml = `<tbody>${bodyRows
        .map(
          (row) =>
            `<tr>${row.map((c) => `<td>${applyInlineMarkdown(c, restoreInline)}</td>`).join("")}</tr>`,
        )
        .join("")}</tbody>`;
      out.push(`<table>${headHtml}${bodyHtml}</table>`);
      continue;
    }

    // Blockquote
    if (/^>\s?/.test(trimmed)) {
      flushPara();
      const quoteLines: string[] = [];
      while (i < lines.length && /^>\s?/.test(lines[i].trim())) {
        quoteLines.push(lines[i].trim().replace(/^>\s?/, ""));
        i++;
      }
      out.push(
        `<blockquote>${quoteLines
          .map((q) => applyInlineMarkdown(q, restoreInline))
          .join("<br />")}</blockquote>`,
      );
      continue;
    }

    // Unordered list
    if (/^[-*+]\s+/.test(trimmed)) {
      flushPara();
      const items: string[] = [];
      while (i < lines.length && /^[-*+]\s+/.test(lines[i].trim())) {
        items.push(lines[i].trim().replace(/^[-*+]\s+/, ""));
        i++;
      }
      out.push(
        `<ul>${items.map((it) => `<li>${applyInlineMarkdown(it, restoreInline)}</li>`).join("")}</ul>`,
      );
      continue;
    }

    // Ordered list
    if (/^\d+\.\s+/.test(trimmed)) {
      flushPara();
      const items: string[] = [];
      while (i < lines.length && /^\d+\.\s+/.test(lines[i].trim())) {
        items.push(lines[i].trim().replace(/^\d+\.\s+/, ""));
        i++;
      }
      out.push(
        `<ol>${items.map((it) => `<li>${applyInlineMarkdown(it, restoreInline)}</li>`).join("")}</ol>`,
      );
      continue;
    }

    // Raw HTML block lines (e.g. <div align="center">, <p>, </a>, <details>)
    if (HTML_BLOCK_START_RE.test(trimmed)) {
      flushPara();
      const opensP = /<p\b[^>]*>/i.test(trimmed) && !/<\/p\s*>/i.test(trimmed);
      const closesP = /<\/p\s*>/i.test(trimmed) && !/<p\b[^>]*>/i.test(trimmed);
      if (opensP) rawPDepth++;
      if (closesP) rawPDepth = Math.max(0, rawPDepth - 1);
      out.push(applyInlineMarkdown(line, restoreInline));
      i++;
      continue;
    }

    paraBuf.push(line);
    i++;
  }

  flushPara();
  return out.join("\n");
}

function RichImage({
  src,
  alt,
  widthAttr,
  inLink,
}: {
  src: string;
  alt: string;
  widthAttr?: string | null;
  inLink?: boolean;
}) {
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    setFailed(false);
  }, [src]);

  if (failed) {
    if (inLink && alt.trim()) {
      return (
        <span className="inline-flex items-center rounded-full border border-[#D5DAEE] bg-white px-2.5 py-0.5 text-xs font-medium text-[#2B3AE8]">
          {alt.trim()}
        </span>
      );
    }
    return null;
  }

  const parsedWidth =
    widthAttr && /^\d+$/.test(widthAttr.trim()) ? Math.min(Number(widthAttr.trim()), 720) : undefined;

  return (
    // eslint-disable-next-line @next/next/no-img-element
    <img
      src={src}
      alt={alt}
      width={parsedWidth}
      loading="lazy"
      fetchPriority="low"
      referrerPolicy="no-referrer"
      onError={() => setFailed(true)}
      className="my-1 inline-block h-auto max-h-[360px] max-w-full rounded-lg align-middle"
    />
  );
}

function isCenteredElement(el: Element): boolean {
  const align = (el.getAttribute("align") || "").toLowerCase();
  if (align === "center") return true;
  const style = (el.getAttribute("style") || "").toLowerCase();
  return /text-align\s*:\s*center/.test(style);
}

function domNodeToReact(node: ChildNode, key: string, inLink = false): ReactNode {
  if (node.nodeType === Node.TEXT_NODE) {
    return node.textContent;
  }
  if (node.nodeType !== Node.ELEMENT_NODE) {
    return null;
  }

  const el = node as Element;
  const tag = el.tagName.toLowerCase();
  if (DROPPED_TAGS.has(tag)) {
    return null;
  }

  const centered = isCenteredElement(el);
  const nextInLink = inLink || tag === "a";
  const children = Array.from(el.childNodes).map((child, idx) =>
    domNodeToReact(child, `${key}-${idx}`, nextInLink),
  );

  switch (tag) {
    case "h1":
      return (
        <h3
          key={key}
          className={`mt-4 mb-2 text-base font-semibold text-[#14161F] first:mt-0 ${centered ? "text-center" : ""}`}
        >
          {children}
        </h3>
      );
    case "h2":
      return (
        <h4
          key={key}
          className={`mt-4 mb-2 text-[15px] font-semibold text-[#14161F] first:mt-0 ${centered ? "text-center" : ""}`}
        >
          {children}
        </h4>
      );
    case "h3":
    case "h4":
    case "h5":
    case "h6":
      return (
        <h5
          key={key}
          className={`mt-3 mb-1.5 text-sm font-semibold text-[#14161F] first:mt-0 ${centered ? "text-center" : ""}`}
        >
          {children}
        </h5>
      );
    case "p":
      return (
        <p key={key} className={`my-2 leading-relaxed first:mt-0 last:mb-0 ${centered ? "text-center" : ""}`}>
          {children}
        </p>
      );
    case "center":
      return (
        <div key={key} className="my-2 text-center">
          {children}
        </div>
      );
    case "div":
    case "section":
    case "article":
    case "figure":
    case "header":
    case "footer":
      return (
        <div key={key} className={`my-2 first:mt-0 last:mb-0 ${centered ? "text-center" : ""}`}>
          {children}
        </div>
      );
    case "a": {
      const href = sanitizeHref(el.getAttribute("href"));
      if (!href) return <Fragment key={key}>{children}</Fragment>;
      return (
        <a
          key={key}
          href={href}
          target="_blank"
          rel="noopener noreferrer"
          className="font-medium text-[#2B3AE8] underline decoration-[#2B3AE8]/30 underline-offset-2 hover:decoration-[#2B3AE8]"
        >
          {children}
        </a>
      );
    }
    case "img": {
      const src = sanitizeImgSrc(el.getAttribute("src"));
      if (!src) return null;
      return (
        <RichImage
          key={key}
          src={src}
          alt={el.getAttribute("alt") || ""}
          widthAttr={el.getAttribute("width")}
          inLink={inLink}
        />
      );
    }
    case "strong":
    case "b":
      return (
        <strong key={key} className="font-semibold text-[#14161F]">
          {children}
        </strong>
      );
    case "em":
    case "i":
      return <em key={key}>{children}</em>;
    case "u":
    case "ins":
      return <u key={key}>{children}</u>;
    case "del":
    case "s":
    case "strike":
      return (
        <del key={key} className="text-[#8A90A6]">
          {children}
        </del>
      );
    case "code":
      return (
        <code
          key={key}
          className="rounded-md bg-white px-1.5 py-0.5 font-mono text-[12px] text-[#14161F]"
        >
          {children}
        </code>
      );
    case "pre":
      return (
        <pre
          key={key}
          className="my-2.5 overflow-x-auto rounded-xl bg-[#14161F] p-3.5 font-mono text-xs leading-relaxed text-[#F5F7FD] [&_code]:bg-transparent [&_code]:p-0 [&_code]:text-inherit"
        >
          {children}
        </pre>
      );
    case "blockquote":
      return (
        <blockquote
          key={key}
          className="my-2.5 border-l-2 border-[#D5DAEE] pl-3.5 text-[#5B6075]"
        >
          {children}
        </blockquote>
      );
    case "ul":
      return (
        <ul key={key} className="my-2 list-disc space-y-1 pl-5">
          {children}
        </ul>
      );
    case "ol":
      return (
        <ol key={key} className="my-2 list-decimal space-y-1 pl-5">
          {children}
        </ol>
      );
    case "li":
      return <li key={key}>{children}</li>;
    case "hr":
      return <hr key={key} className="my-3.5 border-[#E3E6F2]" />;
    case "br":
      return <br key={key} />;
    case "details":
      return (
        <details key={key} className="my-2 rounded-xl bg-white/80 px-3.5 py-2.5">
          {children}
        </details>
      );
    case "summary":
      return (
        <summary key={key} className="cursor-pointer font-medium text-[#14161F]">
          {children}
        </summary>
      );
    case "table":
      return (
        <div key={key} className="my-2.5 overflow-x-auto">
          <table className="w-full border-collapse text-left text-xs">{children}</table>
        </div>
      );
    case "thead":
      return (
        <thead key={key} className="border-b border-[#E3E6F2] text-[#5B6075]">
          {children}
        </thead>
      );
    case "tbody":
      return (
        <tbody key={key} className="divide-y divide-[#EEF0F7]">
          {children}
        </tbody>
      );
    case "tr":
      return <tr key={key}>{children}</tr>;
    case "th":
      return (
        <th key={key} className="px-2.5 py-2 font-semibold text-[#14161F]">
          {children}
        </th>
      );
    case "td":
      return (
        <td key={key} className="px-2.5 py-2 align-top">
          {children}
        </td>
      );
    default:
      return <Fragment key={key}>{children}</Fragment>;
  }
}

export function AddonRichDescription({ content }: { content: string }) {
  const rendered = useMemo(() => {
    const trimmed = content.trim();
    if (!trimmed) return null;
    const html = compileMarkdownToHtml(trimmed);
    if (typeof DOMParser === "undefined") {
      return <p className="whitespace-pre-wrap">{trimmed}</p>;
    }
    const doc = new DOMParser().parseFromString(html, "text/html");
    return Array.from(doc.body.childNodes).map((node, idx) =>
      domNodeToReact(node, `md-${idx}`),
    );
  }, [content]);

  if (!rendered) {
    return <p className="text-xs text-[#8A90A6]">暂无详细介绍。</p>;
  }

  return (
    <div className="rounded-2xl bg-[#F7F8FC] px-5 py-4 text-[13px] leading-relaxed break-words text-[#3D4258]">
      {rendered}
    </div>
  );
}
