import { Component } from '@angular/core';
@Component({ selector: 'app-settings-page', template: `<section class="content-page narrow-page"><header class="page-heading"><div><h1>Settings</h1><p>Company-wide scheduling defaults.</p></div></header><div class="settings-list"><div><span><strong>Time zone</strong><small>Used for every schedule and audit timestamp.</small></span><b>America/Chicago</b></div><div><span><strong>Publishing</strong><small>Workers see changes only after a manager publishes.</small></span><b>Draft review</b></div></div></section>` })
export class SettingsPage {}
