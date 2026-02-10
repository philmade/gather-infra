/**
 * Markdown and Drafty rendering utilities
 */

class MarkdownRenderer {
    constructor() {
        this.md = null;
        this._init();
    }

    _init() {
        if (typeof markdownit !== 'undefined') {
            this.md = markdownit({
                html: false,
                linkify: true,
                typographer: true,
                highlight: (str, lang) => {
                    if (lang && hljs && hljs.getLanguage(lang)) {
                        try {
                            return hljs.highlight(str, { language: lang }).value;
                        } catch (e) {
                            // Silently fail
                        }
                    }
                    return '';
                }
            });
        }
    }

    render(text) {
        if (!text) return '';
        return this.md ? this.md.render(text) : this._escapeHtml(text);
    }

    renderInline(text) {
        if (!text) return '';
        return this.md ? this.md.renderInline(text) : this._escapeHtml(text);
    }

    /**
     * Convert Tinode Drafty format to HTML
     */
    renderDrafty(drafty) {
        if (!drafty) return '';

        // Simple text string
        if (typeof drafty === 'string') {
            return this._processPlainText(drafty);
        }

        // Handle various content formats
        let text = '';

        if (drafty.txt) {
            text = drafty.txt;
        } else if (drafty.content) {
            if (typeof drafty.content === 'string') {
                return this._processPlainText(drafty.content);
            }
            return this.renderDrafty(drafty.content);
        } else if (drafty.text) {
            return this._processPlainText(drafty.text);
        } else if (drafty.head?.mime === 'text/plain' && drafty.content) {
            return this._processPlainText(String(drafty.content));
        } else {
            // Try common text properties
            for (const prop of ['message', 'body', 'data', 'value']) {
                if (drafty[prop]) {
                    if (typeof drafty[prop] === 'string') {
                        return this._processPlainText(drafty[prop]);
                    }
                    if (drafty[prop].txt) {
                        text = drafty[prop].txt;
                        break;
                    }
                }
            }
            if (!text) {
                return `<span class="text-gray-500 italic">[Unknown format]</span>`;
            }
        }

        // No formatting - return plain text
        if (!drafty.fmt || drafty.fmt.length === 0) {
            return this._processPlainText(text);
        }

        // Process Drafty formatting
        const chars = Array.from(text);
        const result = [];
        let pos = 0;
        const fmt = [...drafty.fmt].sort((a, b) => a.at - b.at);

        for (const f of fmt) {
            if (f.at > pos) {
                result.push(this._escapeHtml(chars.slice(pos, f.at).join('')));
            }
            const spanText = chars.slice(f.at, f.at + f.len).join('');
            const entity = drafty.ent?.[f.key];
            result.push(this._formatSpan(spanText, f.tp, entity));
            pos = f.at + f.len;
        }

        if (pos < chars.length) {
            result.push(this._escapeHtml(chars.slice(pos).join('')));
        }

        return result.join('');
    }

    _formatSpan(text, type, entity) {
        const escaped = this._escapeHtml(text);

        switch (type) {
            case 'ST': return `<strong>${escaped}</strong>`;
            case 'EM': return `<em>${escaped}</em>`;
            case 'DL': return `<del>${escaped}</del>`;
            case 'CO': return `<code class="bg-gray-100 dark:bg-gray-700 px-1 rounded text-sm">${escaped}</code>`;
            case 'BR': return '<br>';
            case 'LN':
                if (entity?.data?.url) {
                    return `<a href="${this._escapeHtml(entity.data.url)}" target="_blank" rel="noopener noreferrer" class="text-slack-accent hover:underline">${escaped}</a>`;
                }
                return escaped;
            case 'MN': return `<span class="text-slack-accent font-medium">@${escaped}</span>`;
            case 'HT': return `<span class="text-slack-accent">#${escaped}</span>`;
            case 'IM':
                if (entity?.data) {
                    const src = entity.data.val || entity.data.url;
                    return `<img src="${this._escapeHtml(src)}" alt="${escaped}" class="max-w-full rounded mt-2">`;
                }
                return escaped;
            case 'EX':
                if (entity?.data) {
                    return `<div class="inline-flex items-center space-x-2 bg-gray-100 dark:bg-gray-700 rounded px-2 py-1 mt-1">
                        <svg class="w-4 h-4 text-gray-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 21h10a2 2 0 002-2V9.414a1 1 0 00-.293-.707l-5.414-5.414A1 1 0 0012.586 3H7a2 2 0 00-2 2v14a2 2 0 002 2z"/>
                        </svg>
                        <span class="text-sm">${escaped}</span>
                    </div>`;
                }
                return escaped;
            default:
                return escaped;
        }
    }

    _processPlainText(text) {
        let result = this._escapeHtml(text);

        // URLs to links
        result = result.replace(
            /(https?:\/\/[^\s<]+)/g,
            '<a href="$1" target="_blank" rel="noopener noreferrer" class="text-slack-accent hover:underline">$1</a>'
        );

        // @mentions
        result = result.replace(/@(\w+)/g, '<span class="text-slack-accent font-medium">@$1</span>');

        // Line breaks
        result = result.replace(/\n/g, '<br>');

        return result;
    }

    _escapeHtml(text) {
        const map = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#039;' };
        return String(text).replace(/[&<>"']/g, m => map[m]);
    }

    parseToDrafty(text) {
        return { txt: text };
    }
}

window.markdownRenderer = new MarkdownRenderer();
