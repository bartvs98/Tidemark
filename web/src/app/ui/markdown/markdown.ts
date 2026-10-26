import { Component, computed, inject, input } from '@angular/core';
import { DomSanitizer, SafeHtml } from '@angular/platform-browser';
import DOMPurify from 'dompurify';
import { Marked, Renderer, Tokens } from 'marked';

// Release notes come from other people's repositories, so they're untrusted: render
// with marked, then sanitise with DOMPurify. Links open in a new tab without a referrer,
// inline styles/forms are dropped, and no remote image ever loads: images become links.
const LINK_REL = 'noopener noreferrer nofollow';

DOMPurify.addHook('afterSanitizeAttributes', (node) => {
  if (node.tagName === 'A' && node.getAttribute('href')) {
    node.setAttribute('target', '_blank');
    node.setAttribute('rel', LINK_REL);
  }
});

/**
 * Replaces every image (markdown or raw `<img>`, also in `<picture>`) with a link "Image: alt"
 * to its URL, so nothing loads from a remote host just by opening the notes. Inside an existing
 * link only the text remains (no nested anchors); a non-http(s) source leaves plain text.
 * Edits the sanitised fragment in place and only creates text and validated http(s) links.
 */
function imagesToLinks(frag: DocumentFragment): void {
  for (const img of Array.from(frag.querySelectorAll('img'))) {
    const alt = (img.getAttribute('alt') ?? '').trim();
    const text = alt ? `Image: ${alt}` : 'Image';
    let href: string | undefined;
    try {
      const u = new URL((img.getAttribute('src') ?? '').trim());
      if (u.protocol === 'http:' || u.protocol === 'https:') href = u.href;
    } catch {
      // not an absolute URL: no link
    }
    if (!href || img.closest('a')) {
      img.replaceWith(document.createTextNode(text));
      continue;
    }
    const a = document.createElement('a');
    a.href = href;
    a.target = '_blank';
    a.rel = LINK_REL;
    a.className = 'tm-prose-image';
    a.textContent = text;
    img.replaceWith(a);
  }
  frag.querySelectorAll('source').forEach((el) => el.remove());
  for (const pic of Array.from(frag.querySelectorAll('picture'))) pic.replaceWith(...Array.from(pic.childNodes));
}

// A fragment, not a string: the edits below work on DOMPurify's own output, which is serialised
// once at the end instead of being re-parsed after sanitising (the pattern behind mutation XSS).
const PURIFY = {
  FORBID_TAGS: ['style', 'form', 'input', 'button', 'textarea', 'select', 'iframe'],
  FORBID_ATTR: ['style'],
  RETURN_DOM_FRAGMENT: true as const,
};

/** A source line to highlight (from the API's breaking-change analysis). */
export interface MarkdownFlag {
  /** The markdown source line, trimmed. */
  line: string;
  level: 'none' | 'attention' | 'breaking';
  /** The text that set the level, underlined in the rendered line where it's found. */
  match?: string;
  /** The flagged heading the line inherits its level from. */
  section?: string;
}

const LEVEL_WORD = { attention: 'Attention', breaking: 'Breaking' } as const;

/** Why a line is flagged, for its hover title: "Attention · matched “is removed”". */
function flagWhy(f: MarkdownFlag): string {
  if (f.level === 'none') return '';
  const why = f.section ? `under “${f.section}”` : f.match ? `matched “${f.match}”` : '';
  return why ? `${LEVEL_WORD[f.level]} · ${why}` : '';
}

const escapeAttr = (s: string) => s.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;');

// List and quote markers differ between a source line and the token that renders it
// ("> - foo" vs a list item whose raw text starts "- foo"), so compare without them.
const MARKERS = /^(?:\s|>)*(?:(?:[-*+]|\d+[.)])\s+)?/;
const flagKey = (line: string) => line.replace(MARKERS, '').trim();

interface Mark {
  level: 'attention' | 'breaking';
  /** Indexes into the flags that matched. */
  indexes: number[];
}

/**
 * Renders markdown to sanitised HTML. With a GitHub repo URL, "#123" links to the issue
 * or pull request and "@name" to the user, like on GitHub. Headings, paragraphs and list
 * items holding a flagged line get class "tm-flag-breaking" or "tm-flag-attention", and
 * `data-flag` with the indexes of those flags, space-separated (select with `[data-flag~="2"]`),
 * and a `title` saying why; the text that matched is wrapped in `span.tm-flag-match`.
 */
export function renderMarkdown(markdown: string, githubRepo?: string, flags: MarkdownFlag[] = []): string {
  const md = new Marked({ gfm: true, breaks: false });
  const marks = new Map<string, Mark>();
  flags.forEach((f, i) => {
    const key = f.level === 'none' ? '' : flagKey(f.line);
    if (!key) return;
    const m = marks.get(key);
    if (!m) marks.set(key, { level: f.level as Mark['level'], indexes: [i] });
    else {
      m.indexes.push(i);
      if (f.level === 'breaking') m.level = 'breaking';
    }
  });
  if (marks.size) {
    // Own text only: a list item's first line, not its nested list.
    const markOf = (raw: string, firstLineOnly: boolean): Mark | undefined => {
      const found: Mark = { level: 'attention', indexes: [] };
      const lines = raw.split('\n');
      for (const l of firstLineOnly ? lines.slice(0, 1) : lines) {
        const m = marks.get(flagKey(l));
        if (!m) continue;
        if (m.level === 'breaking') found.level = 'breaking';
        found.indexes.push(...m.indexes);
      }
      return found.indexes.length ? found : undefined;
    };
    const mark = (html: string, m?: Mark) => {
      if (!m) return html;
      const why = [...new Set(m.indexes.map((i) => flagWhy(flags[i])).filter(Boolean))].join('\n');
      const title = why ? ` title="${escapeAttr(why)}"` : '';
      return html.replace(/^<(\w+)/, `<$1 class="tm-flag-${m.level}" data-flag="${m.indexes.join(' ')}"${title}`);
    };
    md.use({
      renderer: {
        heading(this: Renderer, token: Tokens.Heading) {
          return mark(Renderer.prototype.heading.call(this, token), markOf(token.raw, true));
        },
        paragraph(this: Renderer, token: Tokens.Paragraph) {
          return mark(Renderer.prototype.paragraph.call(this, token), markOf(token.raw, false));
        },
        listitem(this: Renderer, token: Tokens.ListItem) {
          return mark(Renderer.prototype.listitem.call(this, token), markOf(token.raw, true));
        },
      },
    });
  }
  if (githubRepo) {
    md.use({
      extensions: [
        {
          name: 'githubRef',
          level: 'inline',
          // Only where a reference can start: after whitespace or "(", never inside words or URLs.
          start: (src: string) => {
            const m = src.match(/(^|[\s(])[#@][\w]/);
            return m ? m.index! + m[1].length : undefined;
          },
          tokenizer: (src: string) => {
            const m = /^#(\d+)\b/.exec(src) ?? /^@([A-Za-z0-9](?:[A-Za-z0-9-]{0,38}))\b/.exec(src);
            return m ? { type: 'githubRef', raw: m[0], text: m[0], ref: m[1] } : undefined;
          },
          renderer: (token: Tokens.Generic) =>
            token['text'].startsWith('#')
              ? `<a href="${githubRepo}/issues/${token['ref']}">${token['text']}</a>`
              : `<a href="https://github.com/${token['ref']}">${token['text']}</a>`,
        },
      ],
    });
  }
  const frag = DOMPurify.sanitize(md.parse(markdown, { async: false }) as string, PURIFY);
  imagesToLinks(frag);
  underlineMatches(frag, flags);
  const out = document.createElement('div');
  out.append(frag);
  return out.innerHTML;
}

/**
 * Wraps the first occurrence of each flag's `match` in its flagged element (own text only, not a
 * nested list) in `span.tm-flag-match`. A match split across formatting (or not in the rendered
 * text at all) stays unmarked; the title still says it. Edits the sanitised fragment and only adds spans.
 */
function underlineMatches(frag: DocumentFragment, flags: MarkdownFlag[]): void {
  if (!flags.some((f) => f.match && f.level !== 'none')) return;
  for (const el of Array.from(frag.querySelectorAll<HTMLElement>('[data-flag]'))) {
    const matches = new Set(
      (el.dataset['flag'] ?? '')
        .split(' ')
        .map((i) => flags[+i]?.match?.toLowerCase())
        .filter((m): m is string => !!m),
    );
    // Own text: not inside a nested list, nor inside a match already wrapped.
    const own = (n: Node) => {
      for (let p = n.parentElement; p && p !== el; p = p.parentElement) {
        if (p.matches('ul, ol, .tm-flag-match')) return NodeFilter.FILTER_REJECT;
      }
      return NodeFilter.FILTER_ACCEPT;
    };
    for (const match of matches) {
      const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT, { acceptNode: own });
      for (let node = walker.nextNode() as Text | null; node; node = walker.nextNode() as Text | null) {
        const at = node.data.toLowerCase().indexOf(match);
        if (at < 0) continue;
        const hit = node.splitText(at);
        hit.splitText(match.length);
        const span = document.createElement('span');
        span.className = 'tm-flag-match';
        hit.replaceWith(span);
        span.append(hit);
        break;
      }
    }
  }
}

/** Sanitised markdown, styled by `.tm-prose`. */
@Component({
  selector: 'tm-markdown',
  template: '',
  host: { class: 'tm-prose block', '[innerHTML]': 'html()' },
})
export class TmMarkdown {
  private readonly sanitizer = inject(DomSanitizer);

  readonly markdown = input.required<string>();
  /** GitHub repository URL, to link #123 and @user. */
  readonly githubRepo = input<string>();
  /** Source lines to highlight as breaking / attention. */
  readonly flags = input<MarkdownFlag[]>([]);

  // Already sanitised by DOMPurify (which keeps target/rel that Angular's sanitiser drops).
  protected readonly html = computed<SafeHtml>(() =>
    this.sanitizer.bypassSecurityTrustHtml(renderMarkdown(this.markdown(), this.githubRepo(), this.flags())),
  );
}
