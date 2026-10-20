import { renderMarkdown } from './markdown';

function dom(html: string): HTMLElement {
  const div = document.createElement('div');
  div.innerHTML = html;
  return div;
}

describe('renderMarkdown', () => {
  it('renders GitHub-flavoured markdown', () => {
    const el = dom(renderMarkdown('## Breaking\n\n- removed `DB_URL`\n\n| a | b |\n|---|---|\n| 1 | 2 |'));
    expect(el.querySelector('h2')?.textContent).toBe('Breaking');
    expect(el.querySelector('li code')?.textContent).toBe('DB_URL');
    expect(el.querySelector('table td')?.textContent).toBe('1');
  });

  it('strips scripts, event handlers, styles and javascript: links', () => {
    const el = dom(
      renderMarkdown(
        '<script>alert(1)</script><img src="x" onerror="alert(2)"><p style="color:red">hi</p>[x](javascript:alert(3))<form><input></form>',
      ),
    );
    expect(el.querySelector('script, form, input')).toBeNull();
    expect(el.querySelector('img, [onerror]')).toBeNull();
    expect(el.querySelector('p')?.getAttribute('style')).toBeNull();
    expect(el.querySelector('a')?.getAttribute('href') ?? '').not.toContain('javascript');
  });

  it('opens links in a new tab without a referrer', () => {
    const el = dom(renderMarkdown('[docs](https://example.com)'));
    const a = el.querySelector('a')!;
    expect(a.getAttribute('target')).toBe('_blank');
    expect(a.getAttribute('rel')).toContain('noreferrer');
  });

  describe('images', () => {
    const links = (el: HTMLElement) => [...el.querySelectorAll('a')];

    it('turns a markdown image into a link', () => {
      const el = dom(renderMarkdown('![the logo](https://example.com/a.png)'));
      expect(el.querySelector('img')).toBeNull();
      const [a] = links(el);
      expect(a.textContent).toBe('Image: the logo');
      expect(a.getAttribute('href')).toBe('https://example.com/a.png');
      expect(a.getAttribute('target')).toBe('_blank');
      expect(a.getAttribute('rel')).toContain('noreferrer');
    });

    it('turns a raw img into a link, with or without alt', () => {
      const el = dom(renderMarkdown('<img src="https://example.com/a.png" alt="shot"> <img src="http://example.com/b.png">'));
      expect(el.querySelector('img')).toBeNull();
      expect(links(el).map((a) => a.textContent)).toEqual(['Image: shot', 'Image']);
    });

    it('keeps an enclosing link and drops the nested anchor', () => {
      const el = dom(renderMarkdown('[![badge](https://example.com/b.svg)](https://example.com/target)'));
      expect(el.querySelector('img')).toBeNull();
      expect(links(el).length).toBe(1);
      expect(links(el)[0].getAttribute('href')).toBe('https://example.com/target');
      expect(links(el)[0].textContent).toBe('Image: badge');
    });

    it('leaves plain text for data: and javascript: sources', () => {
      const el = dom(renderMarkdown('<img src="data:image/png;base64,AAAA" alt="a"> <img src="javascript:alert(1)" alt="b">'));
      expect(el.querySelector('img')).toBeNull();
      expect(links(el).length).toBe(0);
      expect(el.textContent).toContain('Image: a');
      expect(el.textContent).toContain('Image: b');
    });

    it('drops picture and source wrappers and never keeps srcset', () => {
      const el = dom(
        renderMarkdown(
          '<picture><source srcset="https://example.com/dark.png"><img src="https://example.com/l.png" srcset="https://example.com/2x.png 2x" alt="logo"></picture>',
        ),
      );
      expect(el.querySelector('picture, source, img, [srcset]')).toBeNull();
      expect(links(el).map((a) => a.getAttribute('href'))).toEqual(['https://example.com/l.png']);
    });
  });

  it('links #123 and @user for GitHub repos, but not inside words, URLs or code', () => {
    const repo = 'https://github.com/home-assistant/core';
    const el = dom(renderMarkdown('Fixed (#123) by @frenck. See issue#9, https://x.dev/#5, mail me@example.com and `#7`.', repo));
    const hrefs = [...el.querySelectorAll('a')].map((a) => a.getAttribute('href'));
    expect(hrefs).toContain(`${repo}/issues/123`);
    expect(hrefs).toContain('https://github.com/frenck');
    expect(hrefs.filter((h) => h?.includes('/issues/')).length).toBe(1);
    expect(el.querySelector('code')?.textContent).toBe('#7');

    expect(dom(renderMarkdown('see #123')).querySelector('a')).toBeNull(); // no repo, no links
  });
});

describe('renderMarkdown flags', () => {
  const notes = [
    '## ⚠ BREAKING CHANGES',
    '',
    '* the config file moved',
    '  * nested detail',
    '',
    '> ⚠️ Keycloak support was dropped',
    '',
    'Plain paragraph',
  ].join('\n');

  it('marks headings, list items and quoted paragraphs holding a flagged line', () => {
    const el = dom(
      renderMarkdown(notes, undefined, [
        { line: '## ⚠ BREAKING CHANGES', level: 'breaking' },
        { line: '* the config file moved', level: 'breaking' },
        { line: '> ⚠️ Keycloak support was dropped', level: 'attention' },
      ]),
    );
    expect(el.querySelector('h2')?.className).toBe('tm-flag-breaking');
    const items = el.querySelectorAll('li');
    expect(items[0].className).toBe('tm-flag-breaking');
    expect(items[1].className).toBe(''); // the nested item isn't flagged itself
    expect(el.querySelector('blockquote p')?.className).toBe('tm-flag-attention');
    expect(el.querySelectorAll('[class^="tm-flag"]').length).toBe(3);
  });

  it('tags each flagged element with the indexes of its flags', () => {
    const el = dom(
      renderMarkdown(notes, undefined, [
        { line: '## ⚠ BREAKING CHANGES', level: 'breaking' },
        { line: 'not in the notes', level: 'breaking' },
        { line: '- the config file moved', level: 'attention' },
        { line: '* the config file moved', level: 'breaking' }, // same element, other marker
        { line: 'Plain paragraph', level: 'none' },
      ]),
    );
    expect(el.querySelector('h2')?.getAttribute('data-flag')).toBe('0');
    const item = el.querySelector('[data-flag~="3"]')!;
    expect(item.tagName).toBe('LI');
    expect(item.getAttribute('data-flag')).toBe('2 3');
    expect(item.className).toBe('tm-flag-breaking'); // the most severe of its flags
    expect(el.querySelectorAll('[data-flag]').length).toBe(2);
  });

  it('says why in the title and underlines the matched words in the line itself', () => {
    const el = dom(
      renderMarkdown(notes + '\n\n* removed: the **old** API\n  * removed here too', undefined, [
        { line: '## ⚠ BREAKING CHANGES', level: 'breaking', match: 'BREAKING' },
        { line: '* the config file moved', level: 'breaking', section: '⚠ BREAKING <CHANGES>' },
        { line: '> ⚠️ Keycloak support was dropped', level: 'attention', match: 'not in the text' },
        { line: '* removed: the **old** API', level: 'attention', match: 'Removed' },
      ]),
    );
    const h2 = el.querySelector<HTMLElement>('h2')!;
    expect(h2.title).toBe('Breaking · matched “BREAKING”');
    expect(h2.querySelector('.tm-flag-match')?.textContent).toBe('BREAKING');
    expect(h2.textContent).toBe('⚠ BREAKING CHANGES');

    const moved = el.querySelectorAll('li')[0];
    expect(moved.title).toBe('Breaking · under “⚠ BREAKING <CHANGES>”');
    expect(moved.querySelector('.tm-flag-match')).toBeNull();

    // Not found in the rendered text: the title still says it.
    const quote = el.querySelector<HTMLElement>('blockquote p')!;
    expect(quote.title).toBe('Attention · matched “not in the text”');
    expect(quote.querySelector('.tm-flag-match')).toBeNull();

    // Case-insensitive, first occurrence in its own text, never in a nested list.
    const removed = [...el.querySelectorAll('li')].find((li) => li.title.startsWith('Attention'))!;
    const spans = removed.querySelectorAll('.tm-flag-match');
    expect(spans.length).toBe(1);
    expect(spans[0].textContent).toBe('removed');
    expect(removed.querySelector('li')?.textContent).toBe('removed here too');
  });

  it('renders the same without flags', () => {
    expect(renderMarkdown(notes, undefined, [])).toBe(renderMarkdown(notes));
    expect(dom(renderMarkdown(notes)).querySelector('[class^="tm-flag"]')).toBeNull();
  });
});
