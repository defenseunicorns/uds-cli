import { css, html } from 'lit';

import CDSHeaderMenu from '@carbon/web-components/es/components/ui-shell/header-menu.js';

// @customElement('header-icon-menu')
class HeaderIconMenu extends CDSHeaderMenu {
  static styles = css`
    ${CDSHeaderMenu.styles}
  `;

  handleClick() {
    this.expanded = !this.expanded;
  }

  render() {
    const { expanded, menuLabel, triggerContent } = this;

    return html`
      <a
        part="trigger"
        role="button"
        tabindex="0"
        href="javascript:void 0"
        aria-haspopup="menu"
        class=" cds--header__menu-item cds--header__menu-title "
        aria-expanded="${expanded}"
        @click=${this.handleClick}
        style="height: 44px"
      >
        <svg
          xmlns="http://www.w3.org/2000/svg"
          viewBox="0 0 32 32"
          fill="currentColor"
          preserveAspectRatio="xMidYMid meet"
          width="16"
          height="16"
          aria-hidden="true"
          style="padding-right: 10px"
        >
          <path
            fill="none"
            d="M8.0071,24.93A4.9958,4.9958,0,0,1,13,20h6a4.9959,4.9959,0,0,1,4.9929,4.93,11.94,11.94,0,0,1-15.9858,0ZM20.5,12.5A4.5,4.5,0,1,1,16,8,4.5,4.5,0,0,1,20.5,12.5Z"
          ></path>
          <path
            d="M26.7489,24.93A13.9893,13.9893,0,1,0,2,16a13.899,13.899,0,0,0,3.2511,8.93l-.02.0166c.07.0845.15.1567.2222.2392.09.1036.1864.2.28.3008.28.3033.5674.5952.87.87.0915.0831.1864.1612.28.2417.32.2759.6484.5372.99.7813.0441.0312.0832.0693.1276.1006v-.0127a13.9011,13.9011,0,0,0,16,0V27.48c.0444-.0313.0835-.0694.1276-.1006.3412-.2441.67-.5054.99-.7813.0936-.08.1885-.1586.28-.2417.3025-.2749.59-.5668.87-.87.0933-.1006.1894-.1972.28-.3008.0719-.0825.1522-.1547.2222-.2392ZM16,8a4.5,4.5,0,1,1-4.5,4.5A4.5,4.5,0,0,1,16,8ZM8.0071,24.93A4.9957,4.9957,0,0,1,13,20h6a4.9958,4.9958,0,0,1,4.9929,4.93,11.94,11.94,0,0,1-15.9858,0Z"
          ></path>
        </svg>

        ${triggerContent}

        <svg
          focusable="false"
          preserveAspectRatio="xMidYMid meet"
          xmlns="http://www.w3.org/2000/svg"
          fill="currentColor"
          aria-hidden="true"
          width="16"
          height="16"
          viewBox="0 0 16 16"
          part="trigger-icon"
          class="cds--header__menu-arrow"
        >
          <path d="M8 11L3 6 3.7 5.3 8 9.6 12.3 5.3 13 6z"></path>
        </svg>
      </a>

      <ul part="menu-body" class="cds--header__menu" aria-label=${menuLabel}>
        <slot></slot>
      </ul>
    `;
  }
}

customElements.define('header-icon-menu', HeaderIconMenu);
