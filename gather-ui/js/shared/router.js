/**
 * Hash-based SPA Router
 * Usage: new Router({ '/': homePage, '/search': searchPage, '/skill/:id': detailPage })
 */

class Router {
    constructor(routes, notFound) {
        this.routes = routes;
        this.notFound = notFound || (() => {
            document.getElementById('app').innerHTML =
                '<div class="empty-state"><div class="empty-icon">404</div><div class="empty-text">Page not found</div></div>';
        });

        window.addEventListener('hashchange', () => this._resolve());
        // Initial resolve on load
        if (document.readyState === 'loading') {
            document.addEventListener('DOMContentLoaded', () => this._resolve());
        } else {
            this._resolve();
        }
    }

    _resolve() {
        const hash = window.location.hash.slice(1) || '/';
        const [path, queryString] = hash.split('?');
        const query = Object.fromEntries(new URLSearchParams(queryString || ''));

        for (const [pattern, handler] of Object.entries(this.routes)) {
            const params = this._match(pattern, path);
            if (params !== null) {
                handler({ params, query, path });
                return;
            }
        }

        this.notFound({ path, query: {} });
    }

    _match(pattern, path) {
        const patternParts = pattern.split('/').filter(Boolean);
        const pathParts = path.split('/').filter(Boolean);

        if (patternParts.length !== pathParts.length) return null;

        const params = {};
        for (let i = 0; i < patternParts.length; i++) {
            if (patternParts[i].startsWith(':')) {
                params[patternParts[i].slice(1)] = decodeURIComponent(pathParts[i]);
            } else if (patternParts[i] !== pathParts[i]) {
                return null;
            }
        }
        return params;
    }
}

/**
 * Navigate to a hash route
 */
function navigate(path) {
    window.location.hash = path;
}

window.Router = Router;
window.navigate = navigate;
