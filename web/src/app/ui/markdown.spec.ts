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
    expect(el.querySelector('img')?.getAttribute('onerror')).toBeNull();
    expect(el.querySelector('p')?.getAttribute('style')).toBeNull();
    expect(el.querySelector('a')?.getAttribute('href') ?? '').not.toContain('javascript');
  });

  it('opens links in a new tab without a referrer, and images load lazily', () => {
    const el = dom(renderMarkdown('[docs](https://example.com) ![logo](https://example.com/a.png)'));
    const a = el.querySelector('a')!;
    expect(a.getAttribute('target')).toBe('_blank');
    expect(a.getAttribute('rel')).toContain('noreferrer');
    expect(el.querySelector('img')?.getAttribute('loading')).toBe('lazy');
    expect(el.querySelector('img')?.getAttribute('referrerpolicy')).toBe('no-referrer');
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
