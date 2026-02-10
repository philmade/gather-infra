/**
 * Search functionality - user and message search
 */

class Search {
    constructor() {
        // DOM elements
        this.modal = document.getElementById('search-modal');
        this.input = document.getElementById('search-input');
        this.results = document.getElementById('search-results');
        this.searchBtn = document.getElementById('search-btn');
        this.closeBtn = document.getElementById('close-search-btn');

        // State
        this.isOpen = false;
        this.searchTimeout = null;
        this.lastQuery = '';

        // Callbacks
        this.onSearch = null;
        this.onSelect = null;

        this._init();
    }

    _init() {
        // Open search
        this.searchBtn?.addEventListener('click', () => this.open());

        // Close search
        this.closeBtn?.addEventListener('click', () => this.close());

        // Close on backdrop click
        this.modal?.addEventListener('click', (e) => {
            if (e.target === this.modal) {
                this.close();
            }
        });

        // Close on Escape
        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape' && this.isOpen) {
                this.close();
            }
            // Open on Ctrl+K / Cmd+K
            if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
                e.preventDefault();
                this.open();
            }
        });

        // Search input
        this.input?.addEventListener('input', () => {
            this._debounceSearch();
        });

        // Enter to select first result
        this.input?.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                const firstResult = this.results?.querySelector('[data-topic]');
                if (firstResult) {
                    this._selectResult(firstResult.dataset.topic, firstResult.dataset);
                }
            }
            // Arrow navigation
            if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
                e.preventDefault();
                this._navigateResults(e.key === 'ArrowDown' ? 1 : -1);
            }
        });
    }

    /**
     * Open search modal
     */
    open() {
        if (!this.modal) return;

        this.isOpen = true;
        this.modal.classList.remove('hidden');
        this.input?.focus();
    }

    /**
     * Close search modal
     */
    close() {
        if (!this.modal) return;

        this.isOpen = false;
        this.modal.classList.add('hidden');

        // Clear input
        if (this.input) {
            this.input.value = '';
        }
        if (this.results) {
            this.results.innerHTML = '<p class="text-center text-gray-500 py-8">Start typing to search...</p>';
        }
    }

    /**
     * Debounce search
     */
    _debounceSearch() {
        clearTimeout(this.searchTimeout);
        this.searchTimeout = setTimeout(() => {
            this._performSearch();
        }, 300);
    }

    /**
     * Perform search
     */
    async _performSearch() {
        const query = this.input?.value.trim();

        if (!query) {
            if (this.results) {
                this.results.innerHTML = '<p class="text-center text-gray-500 py-8">Start typing to search...</p>';
            }
            return;
        }

        if (query === this.lastQuery) return;
        this.lastQuery = query;

        // Show loading
        if (this.results) {
            this.results.innerHTML = `
                <div class="flex items-center justify-center py-8">
                    <div class="animate-spin rounded-full h-6 w-6 border-2 border-gray-300 border-t-slack-accent"></div>
                </div>
            `;
        }

        // Call search callback
        if (this.onSearch) {
            try {
                const results = await this.onSearch(query);
                this._renderResults(results);
            } catch (err) {
                console.error('Search error:', err);
                if (this.results) {
                    this.results.innerHTML = `
                        <div class="text-center py-8 text-red-500">
                            Search failed: ${err.message}
                        </div>
                    `;
                }
            }
        }
    }

    /**
     * Render search results
     */
    _renderResults(results) {
        if (!this.results) return;

        if (!results || results.length === 0) {
            this.results.innerHTML = `
                <div class="text-center py-8 text-gray-500">
                    No results found for "${this._escapeHtml(this.lastQuery)}"
                </div>
            `;
            return;
        }

        this.results.innerHTML = results.map((result, index) => `
            <div class="flex items-center space-x-3 p-3 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700 cursor-pointer ${index === 0 ? 'bg-gray-50 dark:bg-gray-700/50' : ''}"
                data-topic="${result.topic}"
                data-name="${this._escapeHtml(result.name)}"
                data-type="${result.type || 'user'}">
                <div class="relative">
                    ${result.isBot ? `
                    <div class="w-10 h-10 rounded-lg bg-purple-500 flex items-center justify-center text-white">
                        <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z"/>
                        </svg>
                    </div>
                    ` : `
                    <div class="w-10 h-10 rounded-lg ${result.type === 'group' ? 'bg-slack-accent' : 'bg-gray-400'} flex items-center justify-center text-white">
                        ${result.photo
                            ? `<img src="${result.photo}" class="w-full h-full rounded-lg object-cover">`
                            : result.type === 'group'
                                ? '#'
                                : (result.name || result.topic).charAt(0).toUpperCase()
                        }
                    </div>
                    `}
                    ${result.online ? `<span class="absolute -bottom-0.5 -right-0.5 w-3 h-3 bg-slack-online border-2 border-white dark:border-gray-800 rounded-full"></span>` : ''}
                </div>
                <div class="flex-1 min-w-0">
                    <p class="font-medium text-gray-900 dark:text-white truncate flex items-center">
                        ${result.type === 'group' ? '#' : '@'}${this._escapeHtml(result.name || result.topic)}
                        ${result.isBot ? '<span class="ml-2 px-1.5 py-0.5 text-xs bg-purple-100 text-purple-700 dark:bg-purple-900 dark:text-purple-300 rounded">BOT</span>' : ''}
                    </p>
                    ${result.description ? `<p class="text-sm text-gray-500 truncate">${this._escapeHtml(result.description)}</p>` : ''}
                </div>
                <div class="flex-shrink-0">
                    <svg class="w-5 h-5 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/>
                    </svg>
                </div>
            </div>
        `).join('');

        // Add click handlers
        this.results.querySelectorAll('[data-topic]').forEach(el => {
            el.addEventListener('click', () => {
                this._selectResult(el.dataset.topic, el.dataset);
            });
        });
    }

    /**
     * Navigate results with arrow keys
     */
    _navigateResults(direction) {
        const items = this.results?.querySelectorAll('[data-topic]');
        if (!items || items.length === 0) return;

        const current = this.results?.querySelector('.bg-gray-50, .dark\\:bg-gray-700\\/50');
        let index = current ? Array.from(items).indexOf(current) : -1;

        index += direction;
        if (index < 0) index = items.length - 1;
        if (index >= items.length) index = 0;

        items.forEach((item, i) => {
            item.classList.toggle('bg-gray-50', i === index);
            item.classList.toggle('dark:bg-gray-700/50', i === index);
        });
    }

    /**
     * Select a result
     */
    _selectResult(topic, data) {
        if (this.onSelect) {
            this.onSelect(topic, data);
        }
        this.close();
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
     * Set results externally
     */
    setResults(results) {
        this._renderResults(results);
    }
}

// Export
window.Search = Search;
