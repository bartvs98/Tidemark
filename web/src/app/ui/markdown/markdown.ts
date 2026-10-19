import { Component, computed, inject, input } from '@angular/core';
import { DomSanitizer, SafeHtml } from '@angular/platform-browser';
import DOMPurify from 'dompurify';
import { Marked, Renderer, Tokens } from 'marked';

// Release notes come from other people's repositories, so they're untrusted: render
// with marked, then sanitise with DOMPurify. Links open in a new tab without a referrer,
// images load lazily without a referrer, and inline styles/forms are dropped.
DOMPurify.addHook('afterSanitizeAttributes', (node) => {
  if (node.tagName === 'A' && node.getAttribute('href')) {
    node.setAttribute('target', '_blank');
    node.setAttribute('rel', 'noopener noreferrer nofollow');
  }
  if (node.tagName === 'IMG') {
    node.setAttribute('loading', 'lazy');
    node.setAttribute('decoding', 'async');
    node.setAttribute('referrerpolicy', 'no-referrer');
  }
});

const PURIFY = {
  FORBID_TAGS: ['style', 'form', 'input', 'button', 'textarea', 'select', 'iframe'],
  FORBID_ATTR: ['style'],
};

/** A source line to highlight (from the API's breaking-change analysis). */
export interface MarkdownFlag {
  /** The markdown source line, trimmed. */
  line: string;
  level: 'none' | 'attention' | 'breaking';
}

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
 * `data-flag` with the indexes of those flags, space-separated (select with `[data-flag~="2"]`).
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
    const mark = (html: string, m?: Mark) =>
      m ? html.replace(/^<(\w+)/, `<$1 class="tm-flag-${m.level}" data-flag="${m.indexes.join(' ')}"`) : html;
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
  return DOMPurify.sanitize(md.parse(markdown, { async: false }) as string, PURIFY);
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
