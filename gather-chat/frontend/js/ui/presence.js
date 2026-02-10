/**
 * Presence management - online status tracking
 */

class Presence {
    constructor() {
        // State
        this.users = new Map(); // userId -> { online, lastSeen }
        this.statusTimeouts = new Map();

        // Callbacks
        this.onStatusChange = null;
    }

    /**
     * Update user presence
     */
    updatePresence(pres) {
        if (!pres) return;

        const userId = pres.src || pres.from;
        if (!userId) return;

        const current = this.users.get(userId) || { online: false };

        switch (pres.what) {
            case 'on':
                current.online = true;
                current.lastSeen = new Date();
                break;
            case 'off':
                current.online = false;
                current.lastSeen = new Date();
                break;
            case 'upd':
                // User info updated
                if (pres.dsc) {
                    current.info = pres.dsc;
                }
                break;
            case 'gone':
                // User deleted/banned
                current.online = false;
                current.gone = true;
                break;
            case 'term':
                // Session terminated
                current.online = false;
                break;
            case 'ua':
                // User agent changed
                current.userAgent = pres.ua;
                break;
            case 'acs':
                // Access permissions changed
                current.acs = pres.acs;
                break;
            case 'tags':
                // Tags updated
                current.tags = pres.tags;
                break;
            case 'msg':
                // New message notification
                // Don't change online status
                break;
            case 'read':
            case 'recv':
                // Read/receive receipt
                // Don't change online status
                break;
            case 'kp':
                // Keypress (typing)
                current.online = true;
                current.typing = true;
                this._setTypingTimeout(userId);
                break;
        }

        this.users.set(userId, current);

        // Emit status change
        if (this.onStatusChange) {
            this.onStatusChange(userId, current);
        }
    }

    /**
     * Set typing timeout
     */
    _setTypingTimeout(userId) {
        // Clear existing timeout
        const existing = this.statusTimeouts.get(userId);
        if (existing) {
            clearTimeout(existing);
        }

        // Set new timeout to clear typing status
        const timeout = setTimeout(() => {
            const user = this.users.get(userId);
            if (user) {
                user.typing = false;
                this.users.set(userId, user);
                if (this.onStatusChange) {
                    this.onStatusChange(userId, user);
                }
            }
        }, 3000);

        this.statusTimeouts.set(userId, timeout);
    }

    /**
     * Get user presence status
     */
    getStatus(userId) {
        return this.users.get(userId) || { online: false };
    }

    /**
     * Check if user is online
     */
    isOnline(userId) {
        const status = this.users.get(userId);
        return status ? status.online : false;
    }

    /**
     * Check if user is typing
     */
    isTyping(userId) {
        const status = this.users.get(userId);
        return status ? status.typing : false;
    }

    /**
     * Get all online users
     */
    getOnlineUsers() {
        const online = [];
        this.users.forEach((status, userId) => {
            if (status.online) {
                online.push(userId);
            }
        });
        return online;
    }

    /**
     * Format last seen text
     */
    formatLastSeen(userId) {
        const status = this.users.get(userId);
        if (!status || !status.lastSeen) {
            return 'Unknown';
        }

        if (status.online) {
            return 'Online';
        }

        const now = new Date();
        const lastSeen = new Date(status.lastSeen);
        const diff = now - lastSeen;

        // Less than a minute
        if (diff < 60000) {
            return 'Just now';
        }

        // Less than an hour
        if (diff < 3600000) {
            const minutes = Math.floor(diff / 60000);
            return `${minutes} minute${minutes > 1 ? 's' : ''} ago`;
        }

        // Less than a day
        if (diff < 86400000) {
            const hours = Math.floor(diff / 3600000);
            return `${hours} hour${hours > 1 ? 's' : ''} ago`;
        }

        // Less than a week
        if (diff < 604800000) {
            const days = Math.floor(diff / 86400000);
            return `${days} day${days > 1 ? 's' : ''} ago`;
        }

        // Otherwise show date
        return lastSeen.toLocaleDateString();
    }

    /**
     * Clear all presence data
     */
    clear() {
        // Clear timeouts
        this.statusTimeouts.forEach(timeout => clearTimeout(timeout));
        this.statusTimeouts.clear();

        // Clear user data
        this.users.clear();
    }

    /**
     * Set multiple users' presence
     */
    setMultiple(users) {
        users.forEach(user => {
            if (user.topic || user.user) {
                this.users.set(user.topic || user.user, {
                    online: user.online || false,
                    lastSeen: user.touched || user.updated,
                    info: user.public
                });
            }
        });
    }
}

// Export
window.Presence = Presence;
