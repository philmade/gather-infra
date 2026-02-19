/**
 * Composer component - message input and formatting
 */

class Composer {
    constructor() {
        // DOM elements
        this.input = document.getElementById('message-input');
        this.sendBtn = document.getElementById('send-btn');
        this.attachBtn = document.getElementById('attach-btn');
        this.emojiBtn = document.getElementById('emoji-btn');

        // State
        this.enabled = false;
        this.currentTopic = null;
        this.replyTo = null;
        this.editingMessage = null;
        this.keyPressTimeout = null;

        // Callbacks
        this.onSend = null;
        this.onTyping = null;
        this.onAttach = null;
        this.onEditSave = null;

        // Mention autocomplete data provider (set by App)
        this.mentionDataProvider = null;
        this.mentionAutocomplete = null;

        this._init();
    }

    _init() {
        // Input handlers
        this.input?.addEventListener('input', () => {
            this._autoResize();
            this._updateSendButton();
            this._triggerTyping();
        });

        this.input?.addEventListener('keydown', (e) => {
            // Send on Enter (without Shift)
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                this.send();
            }

            // Handle formatting shortcuts
            if (e.ctrlKey || e.metaKey) {
                if (e.key === 'b') {
                    e.preventDefault();
                    this._wrapSelection('**', '**');
                } else if (e.key === 'i') {
                    e.preventDefault();
                    this._wrapSelection('_', '_');
                } else if (e.key === 'k') {
                    e.preventDefault();
                    this._wrapSelection('[', '](url)');
                }
            }
        });

        // Send button
        this.sendBtn?.addEventListener('click', () => this.send());

        // Attach button
        this.attachBtn?.addEventListener('click', () => this._handleAttach());

        // Emoji button (placeholder for emoji picker)
        this.emojiBtn?.addEventListener('click', () => this._handleEmoji());

        // Formatting toolbar buttons
        document.querySelectorAll('[title*="Bold"]').forEach(btn => {
            btn.addEventListener('click', () => this._wrapSelection('**', '**'));
        });
        document.querySelectorAll('[title*="Italic"]').forEach(btn => {
            btn.addEventListener('click', () => this._wrapSelection('_', '_'));
        });
        document.querySelectorAll('[title*="Code"]').forEach(btn => {
            btn.addEventListener('click', () => this._wrapSelection('`', '`'));
        });
        document.querySelectorAll('[title*="Link"]').forEach(btn => {
            btn.addEventListener('click', () => this._wrapSelection('[', '](url)'));
        });
        document.querySelectorAll('[title*="Mention"]').forEach(btn => {
            btn.addEventListener('click', () => this._insertText('@'));
        });

        // Initialize mention autocomplete if available
        this._initMentionAutocomplete();
    }

    /**
     * Initialize mention autocomplete
     */
    _initMentionAutocomplete() {
        if (!this.input || !window.MentionAutocomplete) return;

        this.mentionAutocomplete = new MentionAutocomplete(
            this.input,
            () => this._getMentionData()
        );
    }

    /**
     * Get mention data from the app
     */
    _getMentionData() {
        if (this.mentionDataProvider) {
            return this.mentionDataProvider();
        }
        return { members: [], agents: [] };
    }

    /**
     * Set the mention data provider (called by App)
     */
    setMentionDataProvider(provider) {
        this.mentionDataProvider = provider;
        if (this.mentionAutocomplete) {
            this.mentionAutocomplete.setDataProvider(() => this._getMentionData());
        }
    }

    /**
     * Enable the composer
     */
    enable(topic) {
        this.enabled = true;
        this.currentTopic = topic;

        if (this.input) {
            this.input.disabled = false;
            this.input.placeholder = `Message ${topic.startsWith('usr') ? '@' : '#'}${topic}`;

            // Restore draft
            const draft = storage.getDraft(topic);
            if (draft) {
                this.input.value = draft;
                this._autoResize();
            }
        }

        this._updateSendButton();
    }

    /**
     * Disable the composer
     */
    disable() {
        // Save draft before disabling
        if (this.currentTopic && this.input?.value) {
            storage.saveDraft(this.currentTopic, this.input.value);
        }

        this.enabled = false;

        if (this.input) {
            this.input.disabled = true;
            this.input.value = '';
            this.input.placeholder = 'Select a group to start messaging';
            this.input.style.height = 'auto';
        }

        if (this.sendBtn) {
            this.sendBtn.disabled = true;
        }

        this.replyTo = null;
        this.currentTopic = null;
    }

    /**
     * Send the message (or save edit)
     */
    send() {
        if (!this.enabled || !this.input) return;

        const text = this.input.value.trim();
        if (!text) return;

        // Handle edit mode
        if (this.editingMessage) {
            if (this.onEditSave) {
                this.onEditSave(this.editingMessage.seq, text);
            }
            this.clearEditMode();
            return;
        }

        // Call send callback
        if (this.onSend) {
            this.onSend(text, this.replyTo);
        }

        // Clear input
        this.input.value = '';
        this._autoResize();
        this._updateSendButton();

        // Clear draft
        if (this.currentTopic) {
            storage.saveDraft(this.currentTopic, '');
        }

        // Clear reply
        this.replyTo = null;
        this._hideReplyPreview();
    }

    /**
     * Set reply context
     */
    setReply(message) {
        // Clear edit mode if active
        if (this.editingMessage) {
            this.clearEditMode();
        }
        this.replyTo = message;
        this._showReplyPreview(message);
        this.focus();
    }

    /**
     * Clear reply context
     */
    clearReply() {
        this.replyTo = null;
        this._hideReplyPreview();
    }

    /**
     * Enter edit mode for a message
     */
    setEditMode(message) {
        // Clear reply if active
        if (this.replyTo) {
            this.clearReply();
        }

        this.editingMessage = message;
        this.setText(message.content || '');
        this._showEditBanner(message);
        this.focus();
    }

    /**
     * Exit edit mode
     */
    clearEditMode() {
        this.editingMessage = null;
        this._hideEditBanner();
        if (this.input) {
            this.input.value = '';
            this._autoResize();
            this._updateSendButton();
        }
    }

    /**
     * Check if in edit mode
     */
    isEditing() {
        return this.editingMessage !== null;
    }

    /**
     * Show edit mode banner
     */
    _showEditBanner(message) {
        this._hideEditBanner();

        const banner = document.createElement('div');
        banner.id = 'edit-mode-banner';
        banner.innerHTML = `
            <div class="flex items-center space-x-2">
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"/>
                </svg>
                <span>Editing message</span>
            </div>
            <button id="cancel-edit" class="text-sm underline hover:no-underline">Cancel</button>
        `;

        const composer = document.getElementById('composer');
        if (composer) {
            composer.insertBefore(banner, composer.firstChild);

            document.getElementById('cancel-edit')?.addEventListener('click', () => {
                this.clearEditMode();
            });
        }
    }

    /**
     * Hide edit mode banner
     */
    _hideEditBanner() {
        const banner = document.getElementById('edit-mode-banner');
        if (banner) {
            banner.remove();
        }
    }

    /**
     * Focus the input
     */
    focus() {
        this.input?.focus();
    }

    /**
     * Get current text
     */
    getText() {
        return this.input?.value || '';
    }

    /**
     * Set text
     */
    setText(text) {
        if (this.input) {
            this.input.value = text;
            this._autoResize();
            this._updateSendButton();
        }
    }

    /**
     * Insert text at cursor
     */
    _insertText(text) {
        if (!this.input) return;

        const start = this.input.selectionStart;
        const end = this.input.selectionEnd;
        const value = this.input.value;

        this.input.value = value.slice(0, start) + text + value.slice(end);
        this.input.selectionStart = this.input.selectionEnd = start + text.length;
        this.input.focus();

        this._autoResize();
        this._updateSendButton();
    }

    /**
     * Wrap selection with prefix/suffix
     */
    _wrapSelection(prefix, suffix) {
        if (!this.input) return;

        const start = this.input.selectionStart;
        const end = this.input.selectionEnd;
        const value = this.input.value;
        const selected = value.slice(start, end);

        const newValue = value.slice(0, start) + prefix + selected + suffix + value.slice(end);
        this.input.value = newValue;

        // Position cursor
        if (selected) {
            this.input.selectionStart = start;
            this.input.selectionEnd = start + prefix.length + selected.length + suffix.length;
        } else {
            this.input.selectionStart = this.input.selectionEnd = start + prefix.length;
        }

        this.input.focus();
        this._autoResize();
        this._updateSendButton();
    }

    /**
     * Auto-resize textarea
     */
    _autoResize() {
        if (!this.input) return;

        this.input.style.height = 'auto';
        this.input.style.height = Math.min(this.input.scrollHeight, 160) + 'px';
    }

    /**
     * Update send button state
     */
    _updateSendButton() {
        if (!this.sendBtn) return;

        const hasText = this.input?.value.trim().length > 0;
        this.sendBtn.disabled = !this.enabled || !hasText;
    }

    /**
     * Trigger typing indicator
     */
    _triggerTyping() {
        if (!this.enabled) return;

        // Debounce typing notifications
        clearTimeout(this.keyPressTimeout);
        this.keyPressTimeout = setTimeout(() => {
            if (this.onTyping) {
                this.onTyping();
            }
        }, 100);
    }

    /**
     * Handle file attachment
     */
    _handleAttach() {
        const input = document.createElement('input');
        input.type = 'file';
        input.multiple = true;

        input.addEventListener('change', (e) => {
            const files = Array.from(e.target.files || []);
            if (files.length > 0 && this.onAttach) {
                this.onAttach(files);
            }
        });

        input.click();
    }

    /**
     * Handle emoji button
     */
    _handleEmoji() {
        // Placeholder - would open emoji picker
        notifications.info('Emoji picker coming soon!');
    }

    /**
     * Show reply preview
     */
    _showReplyPreview(message) {
        // Remove existing preview
        this._hideReplyPreview();

        const preview = document.createElement('div');
        preview.id = 'reply-preview';
        preview.className = 'flex items-center justify-between px-3 py-2 bg-gray-100 dark:bg-gray-700 border-l-4 border-slack-accent rounded-t mb-0';
        preview.innerHTML = `
            <div class="flex items-center space-x-2 min-w-0">
                <svg class="w-4 h-4 text-gray-400 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 10h10a8 8 0 018 8v2M3 10l6 6m-6-6l6-6"/>
                </svg>
                <span class="text-sm text-gray-600 dark:text-gray-300 truncate">
                    Replying to <strong>${message.userName || message.from}</strong>: ${message.content.slice(0, 50)}${message.content.length > 50 ? '...' : ''}
                </span>
            </div>
            <button id="cancel-reply" class="p-1 hover:bg-gray-200 dark:hover:bg-gray-600 rounded">
                <svg class="w-4 h-4 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
                </svg>
            </button>
        `;

        // Insert before composer
        const composer = document.getElementById('composer');
        if (composer) {
            composer.insertBefore(preview, composer.firstChild);

            // Add cancel handler
            document.getElementById('cancel-reply')?.addEventListener('click', () => {
                this.clearReply();
            });
        }
    }

    /**
     * Hide reply preview
     */
    _hideReplyPreview() {
        const preview = document.getElementById('reply-preview');
        if (preview) {
            preview.remove();
        }
    }
}

// Export
window.Composer = Composer;
