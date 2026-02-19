/**
 * Browser notifications and toast messages
 */

class Notifications {
    constructor() {
        this.permission = 'default';
        this.toastContainer = null;
        this._init();
    }

    _init() {
        // Check notification permission
        if ('Notification' in window) {
            this.permission = Notification.permission;
        }

        // Get toast container
        this.toastContainer = document.getElementById('toast-container');
    }

    /**
     * Request notification permission
     */
    async requestPermission() {
        if (!('Notification' in window)) {
            console.log('Browser does not support notifications');
            return false;
        }

        if (this.permission === 'granted') {
            return true;
        }

        try {
            const permission = await Notification.requestPermission();
            this.permission = permission;
            return permission === 'granted';
        } catch (e) {
            console.error('Notification permission error:', e);
            return false;
        }
    }

    /**
     * Show browser notification
     */
    show(title, options = {}) {
        if (this.permission !== 'granted') {
            return null;
        }

        // Don't show if page is focused
        if (document.hasFocus() && !options.force) {
            return null;
        }

        const notification = new Notification(title, {
            icon: options.icon || '/assets/icons/chat.png',
            body: options.body || '',
            tag: options.tag || 'default',
            silent: options.silent || false,
            ...options
        });

        notification.onclick = () => {
            window.focus();
            notification.close();
            if (options.onClick) {
                options.onClick();
            }
        };

        // Auto close after 5 seconds
        setTimeout(() => notification.close(), options.duration || 5000);

        return notification;
    }

    /**
     * Show new message notification
     */
    showMessageNotification(from, message, topic) {
        return this.show(from, {
            body: message,
            tag: `message-${topic}`,
            onClick: () => {
                // This could trigger navigation to the topic
                if (window.app && window.app.selectTopic) {
                    window.app.selectTopic(topic);
                }
            }
        });
    }

    /**
     * Show toast message
     */
    toast(message, type = 'info', duration = 4000) {
        if (!this.toastContainer) {
            this.toastContainer = document.getElementById('toast-container');
            if (!this.toastContainer) {
                console.warn('Toast container not found');
                return;
            }
        }

        const toast = document.createElement('div');
        toast.className = this._getToastClasses(type);
        toast.innerHTML = `
            <div class="flex items-center space-x-2">
                ${this._getToastIcon(type)}
                <span>${this._escapeHtml(message)}</span>
            </div>
            <button class="ml-4 text-current opacity-70 hover:opacity-100" onclick="this.parentElement.remove()">
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
                </svg>
            </button>
        `;

        this.toastContainer.appendChild(toast);

        // Animate in
        requestAnimationFrame(() => {
            toast.classList.remove('translate-x-full', 'opacity-0');
        });

        // Auto remove
        setTimeout(() => {
            toast.classList.add('translate-x-full', 'opacity-0');
            setTimeout(() => toast.remove(), 300);
        }, duration);

        return toast;
    }

    /**
     * Show success toast
     */
    success(message, duration) {
        return this.toast(message, 'success', duration);
    }

    /**
     * Show error toast
     */
    error(message, duration) {
        return this.toast(message, 'error', duration);
    }

    /**
     * Show warning toast
     */
    warning(message, duration) {
        return this.toast(message, 'warning', duration);
    }

    /**
     * Show info toast
     */
    info(message, duration) {
        return this.toast(message, 'info', duration);
    }

    _getToastClasses(type) {
        const base = 'flex items-center justify-between px-4 py-3 rounded-lg shadow-lg transform transition-all duration-300 translate-x-full opacity-0';

        switch (type) {
            case 'success':
                return `${base} bg-green-500 text-white`;
            case 'error':
                return `${base} bg-red-500 text-white`;
            case 'warning':
                return `${base} bg-yellow-500 text-white`;
            default:
                return `${base} bg-gray-800 text-white`;
        }
    }

    _getToastIcon(type) {
        switch (type) {
            case 'success':
                return `<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
                </svg>`;
            case 'error':
                return `<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
                </svg>`;
            case 'warning':
                return `<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"/>
                </svg>`;
            default:
                return `<svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/>
                </svg>`;
        }
    }

    _escapeHtml(text) {
        const map = {
            '&': '&amp;',
            '<': '&lt;',
            '>': '&gt;',
            '"': '&quot;',
            "'": '&#039;'
        };
        return String(text).replace(/[&<>"']/g, m => map[m]);
    }
}

// Export singleton
window.notifications = new Notifications();
