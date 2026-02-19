/**
 * Messages component - handles message display and interactions
 */

class Messages {
    constructor() {
        // DOM elements
        this.container = document.getElementById('messages-container');
        this.messagesArea = document.getElementById('messages-area');
        this.welcomeScreen = document.getElementById('welcome-screen');
        this.topicIcon = document.getElementById('topic-icon');
        this.topicName = document.getElementById('topic-name');
        this.topicDescription = document.getElementById('topic-description');
        this.membersBtn = document.getElementById('members-btn');
        this.typingIndicator = document.getElementById('typing-indicator');
        this.typingText = document.getElementById('typing-text');

        // State
        this.messages = [];
        this.currentTopic = null;
        this.myUserId = null;
        this.isAtBottom = true;
        this.typingUsers = new Map();
        this.typingTimeout = null;
        this.contextMenu = null;
        this.longPressTimer = null;

        // Callbacks
        this.onMessageAction = null;
        this.onLoadMore = null;
        this.onEdit = null;
        this.onReact = null;
        this.onReply = null;
        this.onDelete = null;

        this._init();
    }

    _init() {
        // Scroll handler for auto-scroll detection
        this.messagesArea?.addEventListener('scroll', () => {
            const { scrollTop, scrollHeight, clientHeight } = this.messagesArea;
            this.isAtBottom = scrollHeight - scrollTop - clientHeight < 50;
        });

        // Infinite scroll for loading older messages
        this.messagesArea?.addEventListener('scroll', () => {
            if (this.messagesArea.scrollTop < 100 && this.onLoadMore) {
                this.onLoadMore();
            }
        });
    }

    /**
     * Set current user ID
     */
    setUserId(userId) {
        this.myUserId = userId;
    }

    /**
     * Show the welcome screen
     */
    showWelcome() {
        if (this.welcomeScreen) {
            this.welcomeScreen.classList.remove('hidden');
        }
        if (this.container) {
            this.container.classList.add('hidden');
        }
        this.currentTopic = null;
    }

    /**
     * Show the messages area for a topic
     */
    showTopic(topic, info) {
        if (this.welcomeScreen) {
            this.welcomeScreen.classList.add('hidden');
        }
        if (this.container) {
            this.container.classList.remove('hidden');
        }

        this.currentTopic = topic;

        // Update header
        const isP2P = topic.startsWith('usr');
        if (this.topicIcon) {
            this.topicIcon.textContent = isP2P ? '@' : '#';
        }
        if (this.topicName) {
            this.topicName.textContent = info?.name || topic;
        }
        if (this.topicDescription && info?.description) {
            this.topicDescription.textContent = info.description;
            this.topicDescription.classList.remove('hidden');
        } else if (this.topicDescription) {
            this.topicDescription.classList.add('hidden');
        }

        // Show members button for groups
        if (this.membersBtn) {
            this.membersBtn.classList.toggle('hidden', isP2P);
        }

        // Clear messages
        this.clear();
    }

    /**
     * Update topic header without clearing messages
     */
    updateTopicInfo(info) {
        if (this.topicName && info?.name) {
            this.topicName.textContent = info.name;
        }
        if (this.topicDescription && info?.description) {
            this.topicDescription.textContent = info.description;
            this.topicDescription.classList.remove('hidden');
        }
    }

    /**
     * Clear all messages
     */
    clear() {
        this.messages = [];
        if (this.messagesArea) {
            this.messagesArea.innerHTML = '';
        }
    }

    /**
     * Add multiple messages (for initial load)
     */
    addMessages(messages) {
        messages.forEach(msg => this.addMessage(msg, false));
        this.scrollToBottom();
    }

    /**
     * Add a single message
     */
    addMessage(msg, scroll = true) {
        // Check if message already exists
        if (this.messages.find(m => m.seq === msg.seq)) {
            return;
        }

        this.messages.push(msg);

        // Create message element
        const el = this._createMessageElement(msg);

        // Insert in correct position (sorted by seq)
        const insertBefore = Array.from(this.messagesArea?.children || []).find(child => {
            const seq = parseInt(child.dataset.seq);
            return seq > msg.seq;
        });

        if (insertBefore) {
            this.messagesArea?.insertBefore(el, insertBefore);
        } else {
            this.messagesArea?.appendChild(el);
        }

        // Add date separator if needed
        this._checkDateSeparator(msg);

        // Scroll if at bottom
        if (scroll && this.isAtBottom) {
            this.scrollToBottom();
        }
    }

    _createMessageElement(msg) {
        const isOwn = msg.from === this.myUserId;
        const userName = msg.userName || msg.from || 'Unknown';
        const timestamp = this._formatTime(msg.ts);
        const isBot = msg.isBot || false;
        const isEdited = msg.edited || msg.head?.edited;
        const reactions = msg.reactions || msg.head?.reactions || {};

        const div = document.createElement('div');
        div.className = 'message-item group flex items-start space-x-3 py-2 px-2 -mx-2 rounded hover:bg-gray-50 dark:hover:bg-gray-700/50';
        div.dataset.seq = msg.seq;
        div.dataset.from = msg.from;

        // Check if this is a consecutive message from the same user
        const prevMsg = this.messages.find(m => m.seq === msg.seq - 1);
        const isConsecutive = prevMsg && prevMsg.from === msg.from &&
            (new Date(msg.ts) - new Date(prevMsg.ts)) < 300000; // 5 minutes

        // Avatar HTML - different for bots
        const avatarHtml = isBot
            ? `<div class="w-9 h-9 rounded-lg bg-purple-500 flex items-center justify-center text-white">
                   <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                       <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"/>
                   </svg>
               </div>`
            : `<div class="w-9 h-9 rounded-lg ${isOwn ? 'bg-slack-accent' : 'bg-gray-400'} flex items-center justify-center text-white font-medium text-sm">
                   ${userName.charAt(0).toUpperCase()}
               </div>`;

        // Bot badge HTML
        const botBadge = isBot
            ? '<span class="ml-1 px-1.5 py-0.5 text-xs bg-purple-100 text-purple-700 dark:bg-purple-900 dark:text-purple-300 rounded font-normal">BOT</span>'
            : '';

        // Edited indicator
        const editedIndicator = isEdited
            ? '<span class="message-edited">(edited)</span>'
            : '';

        // Reactions HTML
        const reactionsHtml = this._renderReactions(reactions, msg.seq);

        if (isConsecutive) {
            div.classList.add('consecutive');
            div.innerHTML = `
                <div class="w-9 flex-shrink-0"></div>
                <div class="flex-1 min-w-0">
                    <div class="message-content text-gray-900 dark:text-white break-words">
                        ${this._renderContent(msg.content, msg.rawContent)}${editedIndicator}
                    </div>
                    ${reactionsHtml}
                </div>
                ${this._getMessageActions(msg, isOwn)}
            `;
        } else {
            div.innerHTML = `
                <div class="flex-shrink-0">
                    ${avatarHtml}
                </div>
                <div class="flex-1 min-w-0">
                    <div class="flex items-baseline space-x-2">
                        <span class="font-medium text-gray-900 dark:text-white">${this._escapeHtml(userName)}${botBadge}</span>
                        <span class="text-xs text-gray-500 dark:text-gray-400">${timestamp}</span>
                    </div>
                    <div class="message-content text-gray-900 dark:text-white break-words mt-0.5">
                        ${this._renderContent(msg.content, msg.rawContent)}${editedIndicator}
                    </div>
                    ${reactionsHtml}
                </div>
                ${this._getMessageActions(msg, isOwn)}
            `;
        }

        // Add context menu handlers
        this._addContextMenuHandlers(div, msg, isOwn);

        return div;
    }

    _renderReactions(reactions, seq) {
        if (!reactions || Object.keys(reactions).length === 0) {
            return '';
        }

        const reactionCounts = {};
        const myReactions = new Set();

        // Count reactions
        for (const [userId, userReactions] of Object.entries(reactions)) {
            for (const emoji of userReactions) {
                reactionCounts[emoji] = (reactionCounts[emoji] || 0) + 1;
                if (userId === this.myUserId) {
                    myReactions.add(emoji);
                }
            }
        }

        const badges = Object.entries(reactionCounts).map(([emoji, count]) => {
            const isMyReaction = myReactions.has(emoji);
            return `<button class="reaction-badge ${isMyReaction ? 'my-reaction' : ''}" data-emoji="${emoji}" data-seq="${seq}">
                <span>${emoji}</span>
                <span class="reaction-count">${count}</span>
            </button>`;
        }).join('');

        return `<div class="message-reactions">${badges}</div>`;
    }

    _addContextMenuHandlers(element, msg, isOwn) {
        // Right-click context menu
        element.addEventListener('contextmenu', (e) => {
            e.preventDefault();
            this._showContextMenu(e.clientX, e.clientY, msg, isOwn);
        });

        // Long press for touch devices
        let longPressTimer;
        let touchStartX, touchStartY;

        element.addEventListener('touchstart', (e) => {
            touchStartX = e.touches[0].clientX;
            touchStartY = e.touches[0].clientY;

            longPressTimer = setTimeout(() => {
                this._showContextMenu(touchStartX, touchStartY, msg, isOwn);
            }, 500);
        }, { passive: true });

        element.addEventListener('touchmove', (e) => {
            const dx = Math.abs(e.touches[0].clientX - touchStartX);
            const dy = Math.abs(e.touches[0].clientY - touchStartY);
            if (dx > 10 || dy > 10) {
                clearTimeout(longPressTimer);
            }
        }, { passive: true });

        element.addEventListener('touchend', () => {
            clearTimeout(longPressTimer);
        }, { passive: true });

        // Reaction badge click handlers
        element.querySelectorAll('.reaction-badge').forEach(badge => {
            badge.addEventListener('click', (e) => {
                e.stopPropagation();
                const emoji = badge.dataset.emoji;
                const seq = parseInt(badge.dataset.seq);
                if (this.onReact) {
                    this.onReact(seq, emoji);
                }
            });
        });

        // Action button handlers
        element.querySelectorAll('[data-action]').forEach(btn => {
            btn.addEventListener('click', (e) => {
                e.stopPropagation();
                const action = btn.dataset.action;
                const seq = parseInt(btn.dataset.seq);
                this._handleAction(action, seq, msg);
            });
        });
    }

    _showContextMenu(x, y, msg, isOwn) {
        this._hideContextMenu();

        const menu = document.createElement('div');
        menu.className = 'context-menu';
        menu.id = 'message-context-menu';

        const items = [
            { icon: '↩️', label: 'Reply', action: 'reply' },
            { icon: '😀', label: 'Add reaction', action: 'react' },
            { icon: '📋', label: 'Copy text', action: 'copy' },
        ];

        if (isOwn) {
            items.push({ icon: '✏️', label: 'Edit', action: 'edit' });
            items.push({ type: 'separator' });
            items.push({ icon: '🗑️', label: 'Delete', action: 'delete', danger: true });
        }

        items.forEach(item => {
            if (item.type === 'separator') {
                const sep = document.createElement('div');
                sep.className = 'context-menu-separator';
                menu.appendChild(sep);
            } else {
                const btn = document.createElement('div');
                btn.className = `context-menu-item ${item.danger ? 'danger' : ''}`;
                btn.innerHTML = `<span>${item.icon}</span><span>${item.label}</span>`;
                btn.addEventListener('click', () => {
                    this._hideContextMenu();
                    this._handleAction(item.action, msg.seq, msg);
                });
                menu.appendChild(btn);
            }
        });

        // Position menu
        document.body.appendChild(menu);

        // Adjust position to stay in viewport
        const rect = menu.getBoundingClientRect();
        if (x + rect.width > window.innerWidth) {
            x = window.innerWidth - rect.width - 10;
        }
        if (y + rect.height > window.innerHeight) {
            y = window.innerHeight - rect.height - 10;
        }

        menu.style.left = `${x}px`;
        menu.style.top = `${y}px`;

        // Close on click outside
        setTimeout(() => {
            document.addEventListener('click', this._hideContextMenuHandler);
            document.addEventListener('contextmenu', this._hideContextMenuHandler);
        }, 0);
    }

    _hideContextMenuHandler = () => {
        this._hideContextMenu();
    }

    _hideContextMenu() {
        const menu = document.getElementById('message-context-menu');
        if (menu) {
            menu.remove();
        }
        document.removeEventListener('click', this._hideContextMenuHandler);
        document.removeEventListener('contextmenu', this._hideContextMenuHandler);
    }

    _handleAction(action, seq, msg) {
        switch (action) {
            case 'reply':
                if (this.onReply) this.onReply(msg);
                break;
            case 'edit':
                if (this.onEdit) this.onEdit(msg);
                break;
            case 'react':
                this._showEmojiPicker(seq);
                break;
            case 'copy':
                navigator.clipboard.writeText(msg.content || '');
                window.notifications?.success('Copied to clipboard');
                break;
            case 'delete':
                if (this.onDelete) this.onDelete(msg);
                break;
        }
    }

    _showEmojiPicker(seq) {
        this._hideEmojiPicker();

        const msgEl = this.messagesArea?.querySelector(`[data-seq="${seq}"]`);
        if (!msgEl) return;

        const picker = document.createElement('div');
        picker.className = 'emoji-picker-popover';
        picker.id = 'emoji-picker';

        // Common emoji quick picks
        const emojis = ['👍', '❤️', '😂', '😮', '😢', '🎉', '🔥', '👀'];

        picker.innerHTML = emojis.map(emoji =>
            `<button data-emoji="${emoji}">${emoji}</button>`
        ).join('');

        picker.querySelectorAll('button').forEach(btn => {
            btn.addEventListener('click', () => {
                const emoji = btn.dataset.emoji;
                if (this.onReact) {
                    this.onReact(seq, emoji);
                }
                this._hideEmojiPicker();
            });
        });

        // Position relative to message
        const rect = msgEl.getBoundingClientRect();
        picker.style.position = 'fixed';
        picker.style.left = `${rect.left}px`;
        picker.style.top = `${rect.top - 50}px`;

        document.body.appendChild(picker);

        // Close on click outside
        setTimeout(() => {
            document.addEventListener('click', this._hideEmojiPickerHandler);
        }, 0);
    }

    _hideEmojiPickerHandler = (e) => {
        const picker = document.getElementById('emoji-picker');
        if (picker && !picker.contains(e.target)) {
            this._hideEmojiPicker();
        }
    }

    _hideEmojiPicker() {
        const picker = document.getElementById('emoji-picker');
        if (picker) {
            picker.remove();
        }
        document.removeEventListener('click', this._hideEmojiPickerHandler);
    }

    /**
     * Update a message's reactions display
     */
    updateMessageReactions(seq, reactions) {
        const msgEl = this.messagesArea?.querySelector(`[data-seq="${seq}"]`);
        if (!msgEl) return;

        // Update internal state
        const msg = this.messages.find(m => m.seq === seq);
        if (msg) {
            if (!msg.head) msg.head = {};
            msg.head.reactions = reactions;
        }

        // Update UI
        let reactionsContainer = msgEl.querySelector('.message-reactions');
        const newHtml = this._renderReactions(reactions, seq);

        if (newHtml) {
            if (reactionsContainer) {
                reactionsContainer.outerHTML = newHtml;
            } else {
                const content = msgEl.querySelector('.message-content');
                if (content) {
                    content.insertAdjacentHTML('afterend', newHtml);
                }
            }
        } else if (reactionsContainer) {
            reactionsContainer.remove();
        }

        // Re-attach handlers
        msgEl.querySelectorAll('.reaction-badge').forEach(badge => {
            badge.addEventListener('click', (e) => {
                e.stopPropagation();
                const emoji = badge.dataset.emoji;
                const seq = parseInt(badge.dataset.seq);
                if (this.onReact) {
                    this.onReact(seq, emoji);
                }
            });
        });
    }

    /**
     * Mark a message as edited
     */
    markMessageEdited(seq) {
        const msgEl = this.messagesArea?.querySelector(`[data-seq="${seq}"]`);
        if (!msgEl) return;

        const content = msgEl.querySelector('.message-content');
        if (content && !content.querySelector('.message-edited')) {
            content.insertAdjacentHTML('beforeend', '<span class="message-edited">(edited)</span>');
        }

        // Update internal state
        const msg = this.messages.find(m => m.seq === seq);
        if (msg) {
            msg.edited = true;
        }
    }

    /**
     * Update message content (for edits)
     */
    updateMessageContent(seq, newContent) {
        const msgEl = this.messagesArea?.querySelector(`[data-seq="${seq}"]`);
        if (!msgEl) return;

        const content = msgEl.querySelector('.message-content');
        if (content) {
            content.innerHTML = this._renderContent(newContent) + '<span class="message-edited">(edited)</span>';
        }

        // Update internal state
        const msg = this.messages.find(m => m.seq === seq);
        if (msg) {
            msg.content = newContent;
            msg.edited = true;
        }
    }

    _renderContent(content, rawContent) {
        // Use Drafty renderer if available
        if (rawContent && typeof rawContent === 'object' && window.markdownRenderer) {
            return window.markdownRenderer.renderDrafty(rawContent);
        }

        if (window.markdownRenderer) {
            return window.markdownRenderer.renderDrafty({ txt: content });
        }

        return this._escapeHtml(content || '');
    }

    _getMessageActions(msg, isOwn) {
        return `
            <div class="message-actions flex items-center space-x-1 flex-shrink-0">
                <button class="p-1 hover:bg-gray-200 dark:hover:bg-gray-600 rounded text-gray-400 hover:text-gray-600" title="React" data-action="react" data-seq="${msg.seq}">
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14.828 14.828a4 4 0 01-5.656 0M9 10h.01M15 10h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/>
                    </svg>
                </button>
                <button class="p-1 hover:bg-gray-200 dark:hover:bg-gray-600 rounded text-gray-400 hover:text-gray-600" title="Reply" data-action="reply" data-seq="${msg.seq}">
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 10h10a8 8 0 018 8v2M3 10l6 6m-6-6l6-6"/>
                    </svg>
                </button>
                ${isOwn ? `
                <button class="p-1 hover:bg-gray-200 dark:hover:bg-gray-600 rounded text-gray-400 hover:text-gray-600" title="Edit" data-action="edit" data-seq="${msg.seq}">
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"/>
                    </svg>
                </button>
                <button class="p-1 hover:bg-gray-200 dark:hover:bg-gray-600 rounded text-gray-400 hover:text-red-500" title="Delete" data-action="delete" data-seq="${msg.seq}">
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/>
                    </svg>
                </button>
                ` : ''}
            </div>
        `;
    }

    _checkDateSeparator(msg) {
        const msgDate = new Date(msg.ts).toDateString();
        const prevMessages = this.messages.filter(m => m.seq < msg.seq);

        if (prevMessages.length === 0) {
            this._insertDateSeparator(msg.ts, msg.seq);
            return;
        }

        const prevDate = new Date(prevMessages[prevMessages.length - 1].ts).toDateString();
        if (msgDate !== prevDate) {
            this._insertDateSeparator(msg.ts, msg.seq);
        }
    }

    _insertDateSeparator(ts, beforeSeq) {
        const date = new Date(ts);
        const today = new Date();
        const yesterday = new Date(today);
        yesterday.setDate(yesterday.getDate() - 1);

        let dateText;
        if (date.toDateString() === today.toDateString()) {
            dateText = 'Today';
        } else if (date.toDateString() === yesterday.toDateString()) {
            dateText = 'Yesterday';
        } else {
            dateText = date.toLocaleDateString('en-US', {
                weekday: 'long',
                month: 'long',
                day: 'numeric'
            });
        }

        const separator = document.createElement('div');
        separator.className = 'date-separator flex items-center my-4';
        separator.innerHTML = `
            <div class="flex-1 border-t border-gray-200 dark:border-gray-700"></div>
            <span class="px-3 text-xs text-gray-500 dark:text-gray-400 font-medium">${dateText}</span>
            <div class="flex-1 border-t border-gray-200 dark:border-gray-700"></div>
        `;

        const msgEl = this.messagesArea?.querySelector(`[data-seq="${beforeSeq}"]`);
        if (msgEl) {
            this.messagesArea?.insertBefore(separator, msgEl);
        }
    }

    _formatTime(ts) {
        if (!ts) return '';
        const date = new Date(ts);
        return date.toLocaleTimeString('en-US', {
            hour: 'numeric',
            minute: '2-digit',
            hour12: true
        });
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

    /**
     * Scroll to bottom of messages
     */
    scrollToBottom() {
        if (this.messagesArea) {
            this.messagesArea.scrollTop = this.messagesArea.scrollHeight;
        }
    }

    /**
     * Show typing indicator
     */
    showTyping(userId, userName) {
        this.typingUsers.set(userId, userName || userId);
        this._updateTypingIndicator();

        // Clear after 3 seconds
        clearTimeout(this.typingTimeout);
        this.typingTimeout = setTimeout(() => {
            this.typingUsers.delete(userId);
            this._updateTypingIndicator();
        }, 3000);
    }

    /**
     * Hide typing indicator for a user
     */
    hideTyping(userId) {
        this.typingUsers.delete(userId);
        this._updateTypingIndicator();
    }

    _updateTypingIndicator() {
        if (!this.typingIndicator) return;

        if (this.typingUsers.size === 0) {
            this.typingIndicator.classList.add('hidden');
            return;
        }

        const names = Array.from(this.typingUsers.values());
        let text;

        if (names.length === 1) {
            text = `${names[0]} is typing...`;
        } else if (names.length === 2) {
            text = `${names[0]} and ${names[1]} are typing...`;
        } else {
            text = `${names[0]} and ${names.length - 1} others are typing...`;
        }

        if (this.typingText) {
            this.typingText.textContent = text;
        }
        this.typingIndicator.classList.remove('hidden');
    }

    /**
     * Update message status (delivered/read)
     */
    updateMessageStatus(seq, status) {
        const msgEl = this.messagesArea?.querySelector(`[data-seq="${seq}"]`);
        if (!msgEl) return;

        // Add status indicator
        let statusEl = msgEl.querySelector('.message-status');
        if (!statusEl) {
            statusEl = document.createElement('span');
            statusEl.className = 'message-status text-xs text-gray-400 ml-2';
            const contentEl = msgEl.querySelector('.message-content');
            if (contentEl) {
                contentEl.appendChild(statusEl);
            }
        }

        if (status === 'read') {
            statusEl.innerHTML = `<svg class="w-3 h-3 inline text-slack-accent" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/></svg>`;
        } else if (status === 'delivered') {
            statusEl.innerHTML = `<svg class="w-3 h-3 inline" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/></svg>`;
        }
    }

    /**
     * Remove a message
     */
    removeMessage(seq) {
        const msgEl = this.messagesArea?.querySelector(`[data-seq="${seq}"]`);
        if (msgEl) {
            msgEl.remove();
        }
        this.messages = this.messages.filter(m => m.seq !== seq);
    }

    /**
     * Get message by seq
     */
    getMessage(seq) {
        return this.messages.find(m => m.seq === seq);
    }
}

// Export
window.Messages = Messages;
