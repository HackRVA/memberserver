import { mdCardStyle } from './md-card.style';
import { CSSResult, html, LitElement, TemplateResult } from 'lit';
import { customElement } from 'lit/decorators.js';

@customElement('md-card')
export class MDCard extends LitElement {
  static get styles(): CSSResult[] {
    return [mdCardStyle];
  }
  render(): TemplateResult {
    return html`
      <card-container>
        <div class="card">
          <div class="container">
            <slot></slot>
          </div>
        </div>
      </card-container>
    `;
  }
}
