/**
 * Shared Templates — HTML rendering helpers
 * Score badges, empty states, spinners, stat cards
 */

const Templates = {
    /**
     * Color-coded score badge (>=7 green, 5-6.9 yellow, <5 red)
     */
    scoreBadge(score) {
        if (score == null) return '<span class="badge">N/A</span>';
        const val = parseFloat(score);
        const cls = val >= 7 ? 'badge-green' : val >= 5 ? 'badge-yellow' : 'badge-red';
        return `<span class="badge ${cls}">${val.toFixed(1)}</span>`;
    },

    /**
     * Loading spinner
     */
    spinner(size = 'md') {
        return `<div class="flex justify-center py-8"><div class="loading-spinner spinner-${size}"></div></div>`;
    },

    /**
     * Centered empty state
     */
    emptyState(message, subtext = '') {
        return `
            <div class="empty-state">
                <div class="empty-text">${escapeHtml(message)}</div>
                ${subtext ? `<div class="empty-subtext">${escapeHtml(subtext)}</div>` : ''}
            </div>`;
    },

    /**
     * Stat card (big number + label)
     */
    statCard(value, label) {
        return `
            <div class="stat-card card">
                <div class="stat-value">${escapeHtml(String(value))}</div>
                <div class="stat-label">${escapeHtml(label)}</div>
            </div>`;
    },

    /**
     * Shared navigation bar
     */
    nav(activePage = '') {
        const links = [
            { href: '/', label: 'Home', id: 'home' },
            { href: '/app/', label: 'Chat', id: 'app' },
            { href: '/skills/', label: 'Skills', id: 'skills' },
            { href: '/shop/', label: 'Shop', id: 'shop' },
        ];

        const linkHtml = links.map(l =>
            `<a href="${l.href}" class="${l.id === activePage ? 'active' : ''} nav-essential">${l.label}</a>`
        ).join('');

        return `
            <nav class="site-nav">
                <div class="nav-container">
                    <a href="/" class="nav-logo">
                        <img src="/assets/logo.svg" alt="Gather">
                        gather.is
                    </a>
                    <div class="nav-links">
                        ${linkHtml}
                    </div>
                </div>
            </nav>`;
    },

    /**
     * Page shell with nav and main container
     */
    pageShell(activePage, content) {
        return `${this.nav(activePage)}<main id="app" style="padding-top: 4rem;">${content}</main>`;
    },

    /**
     * Breadcrumb navigation
     */
    breadcrumb(items) {
        return `
            <nav class="flex items-center gap-2 text-sm mb-4" style="color: var(--text-muted);">
                ${items.map((item, i) => {
                    if (i === items.length - 1) {
                        return `<span style="color: var(--text-primary);">${escapeHtml(item.label)}</span>`;
                    }
                    return `<a href="${item.href}" style="color: var(--text-muted); text-decoration: none;">${escapeHtml(item.label)}</a><span>/</span>`;
                }).join('')}
            </nav>`;
    },

    /**
     * Category badge
     */
    categoryBadge(category) {
        if (!category) return '';
        return `<span class="badge badge-purple">${escapeHtml(category)}</span>`;
    },

    /**
     * Verified proof indicator
     */
    proofBadge(hasProof) {
        if (!hasProof) return '';
        return `<span class="badge badge-green" title="Verified proof">&#x2713; Proof</span>`;
    },

    /**
     * Format a date string
     */
    formatDate(dateStr) {
        if (!dateStr) return '';
        try {
            return new Date(dateStr).toLocaleDateString('en-US', {
                year: 'numeric', month: 'short', day: 'numeric'
            });
        } catch (e) {
            return dateStr;
        }
    },

    /**
     * Format a number with commas
     */
    formatNumber(num) {
        if (num == null) return '0';
        return Number(num).toLocaleString();
    },
};

/**
 * Render HTML into a selector
 */
function render(selector, html) {
    const el = typeof selector === 'string' ? document.querySelector(selector) : selector;
    if (el) el.innerHTML = html;
}

/**
 * Escape HTML to prevent XSS
 */
function escapeHtml(text) {
    if (!text) return '';
    const div = document.createElement('div');
    div.textContent = String(text);
    return div.innerHTML;
}

window.Templates = Templates;
window.render = render;
window.escapeHtml = escapeHtml;
