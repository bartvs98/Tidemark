import { Component, computed, inject, input } from '@angular/core';
import { DomSanitizer, SafeHtml } from '@angular/platform-browser';
import DOMPurify from 'dompurify';
import { Marked, Tokens } from 'marked';

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

/**
 * Renders markdown to sanitised HTML. With a GitHub repo URL, "#123" links to the issue
 * or pull request and "@name" to the user, like on GitHub.
 */
export function renderMarkdown(markdown: string, githubRepo?: string): string {
  const md = new Marked({ gfm: true, breaks: false });
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

  // Already sanitised by DOMPurify (which keeps target/rel that Angular's sanitiser drops).
  protected readonly html = computed<SafeHtml>(() =>
    this.sanitizer.bypassSecurityTrustHtml(renderMarkdown(this.markdown(), this.githubRepo())),
  );
}
