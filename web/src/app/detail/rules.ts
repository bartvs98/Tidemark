import { ContainerRules, RuleKey, ruleOrigin } from '../dashboard/containers';

/** One active rule in plain words: `text` then `value` (in mono), plus where it was set. */
export interface RuleLine {
  key: RuleKey;
  text: string;
  value?: string;
  note?: string;
  origin?: { kind: 'label' | 'config'; text: string; label?: string };
}

/** The active rules, in a fixed order, described for the Rules section of the detail page. */
export function describeRules(rules: ContainerRules | undefined): RuleLine[] {
  if (!rules) return [];
  const lines: Omit<RuleLine, 'origin'>[] = [];
  if (rules.ignore) lines.push({ key: 'ignore', text: 'Ignored', note: 'Not checked for updates.' });
  if (rules.pin) {
    lines.push({ key: 'pin', text: 'Pinned to', value: rules.pin, note: `Only versions starting with ${rules.pin} count as updates.` });
  }
  if (rules.allowMajor === false) {
    lines.push({ key: 'allow-major', text: 'Major updates off', note: 'Only versions in the running major count as updates.' });
  }
  if (rules.prerelease) lines.push({ key: 'prerelease', text: 'Pre-releases on', note: 'Betas and release candidates count as updates.' });
  if (rules.tagPattern) lines.push({ key: 'tag-pattern', text: 'Only tags matching', value: rules.tagPattern });
  if (rules.repo) lines.push({ key: 'repo', text: 'Release notes from', value: rules.repo });
  return lines.map((l) => ({ ...l, origin: ruleOrigin(rules, l.key) }));
}
