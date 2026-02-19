/**
 * Local storage helpers for persistent state
 */

class Storage {
    constructor(prefix = 'tinode_chat_') {
        this.prefix = prefix;
    }

    /**
     * Get a value from storage
     */
    get(key, defaultValue = null) {
        try {
            const item = localStorage.getItem(this.prefix + key);
            if (item === null) return defaultValue;
            return JSON.parse(item);
        } catch (e) {
            console.error('Storage get error:', e);
            return defaultValue;
        }
    }

    /**
     * Set a value in storage
     */
    set(key, value) {
        try {
            localStorage.setItem(this.prefix + key, JSON.stringify(value));
            return true;
        } catch (e) {
            console.error('Storage set error:', e);
            return false;
        }
    }

    /**
     * Remove a value from storage
     */
    remove(key) {
        try {
            localStorage.removeItem(this.prefix + key);
            return true;
        } catch (e) {
            console.error('Storage remove error:', e);
            return false;
        }
    }

    /**
     * Clear all app storage
     */
    clear() {
        try {
            const keysToRemove = [];
            for (let i = 0; i < localStorage.length; i++) {
                const key = localStorage.key(i);
                if (key && key.startsWith(this.prefix)) {
                    keysToRemove.push(key);
                }
            }
            keysToRemove.forEach(key => localStorage.removeItem(key));
            return true;
        } catch (e) {
            console.error('Storage clear error:', e);
            return false;
        }
    }

    // Convenience methods for common data

    /**
     * Save authentication token
     */
    saveAuthToken(token) {
        return this.set('auth_token', token);
    }

    /**
     * Get authentication token
     */
    getAuthToken() {
        return this.get('auth_token');
    }

    /**
     * Save last active topic
     */
    saveLastTopic(topicName) {
        return this.set('last_topic', topicName);
    }

    /**
     * Get last active topic
     */
    getLastTopic() {
        return this.get('last_topic');
    }

    /**
     * Save user preferences
     */
    savePreferences(prefs) {
        const current = this.get('preferences', {});
        return this.set('preferences', { ...current, ...prefs });
    }

    /**
     * Get user preferences
     */
    getPreferences() {
        return this.get('preferences', {
            darkMode: false,
            notifications: true,
            sounds: true,
            compactMode: false
        });
    }

    /**
     * Save draft message for a topic
     */
    saveDraft(topicName, text) {
        const drafts = this.get('drafts', {});
        if (text) {
            drafts[topicName] = text;
        } else {
            delete drafts[topicName];
        }
        return this.set('drafts', drafts);
    }

    /**
     * Get draft message for a topic
     */
    getDraft(topicName) {
        const drafts = this.get('drafts', {});
        return drafts[topicName] || '';
    }

    /**
     * Save sidebar collapsed state
     */
    saveSidebarState(section, collapsed) {
        const state = this.get('sidebar_state', {});
        state[section] = collapsed;
        return this.set('sidebar_state', state);
    }

    /**
     * Get sidebar collapsed state
     */
    getSidebarState(section) {
        const state = this.get('sidebar_state', {});
        return state[section] || false;
    }

    /**
     * Save message cache for a topic (backup for persistence)
     * Keeps last N messages per topic
     */
    saveMessageCache(topic, messages) {
        const caches = this.get('message_caches', {});
        // Keep last 30 messages per topic
        const recent = messages.slice(-30).map(msg => ({
            seq: msg.seq,
            from: msg.from,
            content: msg.content,
            ts: msg.ts,
            isOwn: msg.isOwn,
            userName: msg.userName,
            isBot: msg.isBot
        }));
        caches[topic] = {
            messages: recent,
            savedAt: Date.now()
        };
        // Limit to 20 topics to prevent storage bloat
        const topicKeys = Object.keys(caches);
        if (topicKeys.length > 20) {
            // Remove oldest caches
            topicKeys
                .sort((a, b) => (caches[a].savedAt || 0) - (caches[b].savedAt || 0))
                .slice(0, topicKeys.length - 20)
                .forEach(key => delete caches[key]);
        }
        return this.set('message_caches', caches);
    }

    /**
     * Get cached messages for a topic
     */
    getMessageCache(topic) {
        const caches = this.get('message_caches', {});
        const cache = caches[topic];
        if (!cache) return [];
        // Only return if cache is less than 24 hours old
        const maxAge = 24 * 60 * 60 * 1000;
        if (Date.now() - (cache.savedAt || 0) > maxAge) {
            return [];
        }
        return cache.messages || [];
    }

    /**
     * Clear message cache for a topic
     */
    clearMessageCache(topic) {
        const caches = this.get('message_caches', {});
        if (caches[topic]) {
            delete caches[topic];
            return this.set('message_caches', caches);
        }
        return true;
    }
}

// Export singleton
window.storage = new Storage();
