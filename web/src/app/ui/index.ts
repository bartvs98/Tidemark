import { Tab, TabContent, TabList, TabPanel, Tabs } from '@angular/aria/tabs';

export * from './app-icon/app-icon';
export * from './badge/badge';
export * from './button/button';
export * from './callout/callout';
export * from './card/card';
export * from './clamp/clamp';
export * from './dialog/dialog';
export * from './empty-state/empty-state';
export * from './icon/icon';
export * from './section/section';
export * from './segmented/segmented';
export * from './skeleton/skeleton';
export * from './update-class/update-class';
export * from './level/level';
export * from './version-diff/version-diff';
export * from './theme-switch/theme';
export * from './theme-switch/theme-switch';
export * from './tide-backdrop/tide-backdrop';
export * from './tide-gauge/tide-gauge';
export * from './toaster/toasts';
export * from './toaster/toaster';
export * from './tooltip/tooltip';

// Not re-exported: ./markdown/markdown pulls in marked + DOMPurify and registers a sanitiser hook,
// so it would land in the main bundle. Import it directly (from lazy routes only).

/**
 * Tabs are Angular Aria's headless directives (keyboard nav, roles, focus) styled by the
 * `tm-tabs` class in styles/components.css:
 *
 *   <div ngTabs class="tm-tabs">
 *     <div ngTabList selectedTab="changes">
 *       <button ngTab value="changes">Changes</button>
 *     </div>
 *     <div ngTabPanel value="changes"><ng-template ngTabContent>…</ng-template></div>
 *   </div>
 */
export const TM_TABS = [Tabs, TabList, Tab, TabPanel, TabContent] as const;
