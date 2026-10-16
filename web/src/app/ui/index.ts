import { Tab, TabContent, TabList, TabPanel, Tabs } from '@angular/aria/tabs';

export * from './badge';
export * from './button';
export * from './card';
export * from './empty-state';
export * from './icon';
export * from './section';
export * from './segmented';
export * from './skeleton';
export * from './status';
export * from './theme';
export * from './toast';

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
