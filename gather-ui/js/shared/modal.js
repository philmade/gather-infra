/**
 * ModalManager — Show/hide/toggle modal overlays
 * Replaces duplicate show/hide patterns in app.js
 */

class ModalManager {
    constructor(modalId) {
        this.el = document.getElementById(modalId);
        if (!this.el) return;

        // Click outside to close
        this.el.addEventListener('click', (e) => {
            if (e.target === this.el) this.hide();
        });

        // Escape key to close
        this._onKeyDown = (e) => {
            if (e.key === 'Escape' && !this.el.classList.contains('hidden')) {
                this.hide();
            }
        };
        document.addEventListener('keydown', this._onKeyDown);
    }

    show() {
        this.el?.classList.remove('hidden');
    }

    hide() {
        this.el?.classList.add('hidden');
    }

    toggle() {
        this.el?.classList.toggle('hidden');
    }

    get isVisible() {
        return this.el && !this.el.classList.contains('hidden');
    }

    /**
     * Attach a submit handler to a form inside this modal
     */
    onSubmit(formId, handler) {
        const form = document.getElementById(formId);
        if (form) {
            form.addEventListener('submit', (e) => {
                e.preventDefault();
                handler(e);
            });
        }
    }

    /**
     * Wire a close button
     */
    closeOn(buttonId) {
        document.getElementById(buttonId)?.addEventListener('click', () => this.hide());
    }

    destroy() {
        document.removeEventListener('keydown', this._onKeyDown);
    }
}

window.ModalManager = ModalManager;
