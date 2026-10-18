import { Tab, TabContent, TabList, TabPanel, Tabs } from '@angular/aria/tabs';

export * from './app-icon';
export * from './badge';
export * from './button';
export * from './callout';
export * from './card';
export * from './clamp';
export * from './dialog';
export * from './empty-state';
export * from './icon';
export * from './section';
export * from './segmented';
export * from './skeleton';
export * from './status';
export * from './theme';
export * from './tide-backdrop';
export * from './tide-gauge';
export * from './toast';
export * from './tooltip';

// Not re-exported: ./markdown pulls in marked + DOMPurify and registers a sanitiser hook,
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
