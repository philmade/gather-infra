/**
 * Mention Autocomplete Component
 * Provides @mention suggestions for users and agents in the composer
 */

class MentionAutocomplete {
    constructor(inputElement, dataProvider) {
        this.input = inputElement;
        this.dataProvider = dataProvider; // Function returning { members: [], agents: [] }
        this.dropdown = null;
        this.isOpen = false;
        this.selectedIndex = 0;
        this.suggestions = [];
        this.mentionStart = -1;
        this.query = '';

        this._init();
    }

    _init() {
        // Create dropdown element
        this._createDropdown();

        // Listen for input changes
        this.input.addEventListener('input', (e) => this._onInput(e));
        this.input.addEventListener('keydown', (e) => this._onKeyDown(e));
        this.input.addEventListener('blur', () => {
            // Delay to allow click on dropdown
            setTimeout(() => this._close(), 150);
        });

        // Close on click outside
        document.addEventListener('click', (e) => {
            if (!this.dropdown.contains(e.target) && e.target !== this.input) {
                this._close();
            }
        });
    }

    _createDropdown() {
        this.dropdown = document.createElement('div');
        this.dropdown.className = 'mention-dropdown absolute bottom-full left-0 mb-1 w-64 max-h-60 overflow-y-auto bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-600 rounded-lg shadow-lg z-50 hidden';

        // Insert dropdown near the composer
        const composer = document.getElementById('composer');
        if (composer) {
            composer.style.position = 'relative';
            composer.appendChild(this.dropdown);
        }
    }

    _onInput(e) {
        const value = this.input.value;
        const cursorPos = this.input.selectionStart;

        // Find @ symbol before cursor
        let atIndex = -1;
        for (let i = cursorPos - 1; i >= 0; i--) {
            const char = value[i];
            if (char === '@') {
                // Check if @ is at start or preceded by whitespace
                if (i === 0 || /\s/.test(value[i - 1])) {
                    atIndex = i;
                    break;
                }
            }
            // Stop if we hit whitespace (no @ in this word)
            if (/\s/.test(char)) {
                break;
            }
        }

        if (atIndex >= 0) {
            this.mentionStart = atIndex;
            this.query = value.slice(atIndex + 1, cursorPos).toLowerCase();
            this._showSuggestions();
        } else {
            this._close();
        }
    }

    _onKeyDown(e) {
        if (!this.isOpen) return;

        switch (e.key) {
            case 'ArrowDown':
                e.preventDefault();
                this.selectedIndex = Math.min(this.selectedIndex + 1, this.suggestions.length - 1);
                this._updateSelection();
                break;
            case 'ArrowUp':
                e.preventDefault();
                this.selectedIndex = Math.max(this.selectedIndex - 1, 0);
                this._updateSelection();
                break;
            case 'Enter':
            case 'Tab':
                if (this.suggestions.length > 0) {
                    e.preventDefault();
                    this._selectSuggestion(this.suggestions[this.selectedIndex]);
                }
                break;
            case 'Escape':
                e.preventDefault();
                this._close();
                break;
        }
    }

    async _showSuggestions() {
        const data = await this.dataProvider();
        const members = data.members || [];
        const agents = data.agents || [];

        // Combine and filter suggestions
        const allSuggestions = [];

        // Add agents first (they use @agent_name format)
        agents.forEach(agent => {
            const handle = agent.handle || '';
            const displayName = agent.displayName || handle;
            if (handle.toLowerCase().includes(this.query) || displayName.toLowerCase().includes(this.query)) {
                allSuggestions.push({
                    type: 'agent',
                    id: agent.tinodeUserId,
                    handle: handle,
                    name: displayName,
                    online: agent.status === 'running' || agent.status === 'dev',
                    isBot: true
                });
            }
        });

        // Add regular members
        members.forEach(m => {
            // Skip if already added as agent
            if (allSuggestions.some(s => s.id === m.id)) return;

            const name = m.name || m.id || '';
            const handle = this._nameToHandle(name);
            if (name.toLowerCase().includes(this.query) || handle.toLowerCase().includes(this.query)) {
                allSuggestions.push({
                    type: 'user',
                    id: m.id,
                    handle: handle,
                    name: name,
                    online: m.online,
                    isBot: m.isBot
                });
            }
        });

        this.suggestions = allSuggestions.slice(0, 10); // Limit to 10 suggestions
        this.selectedIndex = 0;

        if (this.suggestions.length > 0) {
            this._renderSuggestions();
            this._open();
        } else {
            this._close();
        }
    }

    _nameToHandle(name) {
        // Convert display name to handle format (lowercase, underscores)
        return (name || '')
            .toLowerCase()
            .replace(/\s+/g, '_')
            .replace(/[^a-z0-9_]/g, '');
    }

    _renderSuggestions() {
        this.dropdown.innerHTML = this.suggestions.map((s, i) => `
            <div class="mention-item flex items-center space-x-2 px-3 py-2 cursor-pointer ${i === this.selectedIndex ? 'bg-slack-accent text-white' : 'hover:bg-gray-100 dark:hover:bg-gray-700'}"
                 data-index="${i}">
                <div class="relative flex-shrink-0">
                    ${s.isBot ? `
                        <div class="w-6 h-6 rounded bg-purple-500 flex items-center justify-center text-white text-xs">
                            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"/>
                            </svg>
                        </div>
                    ` : `
                        <div class="w-6 h-6 rounded bg-gray-400 flex items-center justify-center text-white text-xs font-medium">
                            ${(s.name || s.handle).charAt(0).toUpperCase()}
                        </div>
                    `}
                    ${s.online ? `
                        <span class="absolute -bottom-0.5 -right-0.5 w-2 h-2 bg-slack-online border border-white dark:border-gray-800 rounded-full"></span>
                    ` : ''}
                </div>
                <div class="flex-1 min-w-0">
                    <div class="text-sm font-medium truncate ${i === this.selectedIndex ? 'text-white' : 'text-gray-900 dark:text-white'}">
                        @${s.handle}
                    </div>
                    ${s.name !== s.handle ? `
                        <div class="text-xs truncate ${i === this.selectedIndex ? 'text-white/70' : 'text-gray-500 dark:text-gray-400'}">
                            ${s.name}
                        </div>
                    ` : ''}
                </div>
                ${s.isBot ? `
                    <span class="flex-shrink-0 px-1.5 py-0.5 text-xs rounded ${i === this.selectedIndex ? 'bg-white/20 text-white' : 'bg-purple-100 text-purple-700 dark:bg-purple-900 dark:text-purple-300'}">
                        BOT
                    </span>
                ` : ''}
            </div>
        `).join('');

        // Add click handlers
        this.dropdown.querySelectorAll('.mention-item').forEach(item => {
            item.addEventListener('mousedown', (e) => {
                e.preventDefault(); // Prevent blur
                const index = parseInt(item.dataset.index);
                this._selectSuggestion(this.suggestions[index]);
            });
            item.addEventListener('mouseenter', () => {
                this.selectedIndex = parseInt(item.dataset.index);
                this._updateSelection();
            });
        });
    }

    _updateSelection() {
        this.dropdown.querySelectorAll('.mention-item').forEach((item, i) => {
            if (i === this.selectedIndex) {
                item.classList.add('bg-slack-accent', 'text-white');
                item.classList.remove('hover:bg-gray-100', 'dark:hover:bg-gray-700');
                // Update inner text colors
                item.querySelectorAll('.text-gray-900, .dark\\:text-white').forEach(el => {
                    el.classList.remove('text-gray-900', 'dark:text-white');
                    el.classList.add('text-white');
                });
                item.querySelectorAll('.text-gray-500, .dark\\:text-gray-400').forEach(el => {
                    el.classList.remove('text-gray-500', 'dark:text-gray-400');
                    el.classList.add('text-white/70');
                });
            } else {
                item.classList.remove('bg-slack-accent', 'text-white');
                item.classList.add('hover:bg-gray-100', 'dark:hover:bg-gray-700');
            }
        });
    }

    _selectSuggestion(suggestion) {
        if (!suggestion) return;

        const value = this.input.value;
        const beforeMention = value.slice(0, this.mentionStart);
        const afterMention = value.slice(this.input.selectionStart);

        // Insert @handle with a trailing space
        const mention = `@${suggestion.handle} `;
        this.input.value = beforeMention + mention + afterMention;

        // Position cursor after the mention
        const newPos = this.mentionStart + mention.length;
        this.input.selectionStart = this.input.selectionEnd = newPos;

        // Trigger input event for any listeners (like auto-resize)
        this.input.dispatchEvent(new Event('input', { bubbles: true }));

        this._close();
        this.input.focus();
    }

    _open() {
        this.isOpen = true;
        this.dropdown.classList.remove('hidden');
    }

    _close() {
        this.isOpen = false;
        this.dropdown.classList.add('hidden');
        this.mentionStart = -1;
        this.query = '';
        this.suggestions = [];
    }

    /**
     * Update the data provider
     */
    setDataProvider(provider) {
        this.dataProvider = provider;
    }

    /**
     * Destroy the autocomplete
     */
    destroy() {
        if (this.dropdown && this.dropdown.parentNode) {
            this.dropdown.parentNode.removeChild(this.dropdown);
        }
    }
}

// Export
window.MentionAutocomplete = MentionAutocomplete;
