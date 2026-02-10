/**
 * API Client — Fetch wrapper for all Gather API endpoints
 * Auto-injects PocketBase auth token, handles errors
 */

class APIClient {
    constructor() {
        this.baseUrl = this._detectBaseUrl();
    }

    _detectBaseUrl() {
        const hostname = window.location.hostname;
        if (hostname === 'localhost' || hostname === '127.0.0.1') {
            return 'http://localhost:8090';
        }
        return window.location.origin;
    }

    _getAuthHeaders() {
        const headers = { 'Content-Type': 'application/json' };
        // Try PocketBase auth token
        const pbAuth = localStorage.getItem('pocketbase_auth');
        if (pbAuth) {
            try {
                const parsed = JSON.parse(pbAuth);
                if (parsed.token) {
                    headers['Authorization'] = `Bearer ${parsed.token}`;
                }
            } catch (e) { /* ignore */ }
        }
        return headers;
    }

    async _fetch(path, options = {}) {
        const url = `${this.baseUrl}${path}`;
        const response = await fetch(url, {
            headers: this._getAuthHeaders(),
            ...options,
        });

        if (!response.ok) {
            const errorBody = await response.text();
            let message;
            try {
                const parsed = JSON.parse(errorBody);
                message = parsed.title || parsed.message || parsed.detail || `HTTP ${response.status}`;
            } catch (e) {
                message = `HTTP ${response.status}: ${errorBody.substring(0, 200)}`;
            }
            throw new Error(message);
        }

        return response.json();
    }

    async _post(path, body) {
        return this._fetch(path, {
            method: 'POST',
            body: JSON.stringify(body),
        });
    }

    // === Skills ===

    async getSkills(params = {}) {
        const qs = new URLSearchParams();
        if (params.q) qs.set('q', params.q);
        if (params.category) qs.set('category', params.category);
        if (params.sort) qs.set('sort', params.sort);
        if (params.limit) qs.set('limit', params.limit);
        if (params.offset) qs.set('offset', params.offset);
        if (params.min_security) qs.set('min_security', params.min_security);
        const query = qs.toString();
        return this._fetch(`/api/skills${query ? '?' + query : ''}`);
    }

    async getSkill(id) {
        return this._fetch(`/api/skills/${encodeURIComponent(id)}`);
    }

    // === Reviews ===

    async getReviews(params = {}) {
        const qs = new URLSearchParams();
        if (params.skill_id) qs.set('skill_id', params.skill_id);
        if (params.limit) qs.set('limit', params.limit);
        const query = qs.toString();
        return this._fetch(`/api/reviews${query ? '?' + query : ''}`);
    }

    async getReview(id) {
        return this._fetch(`/api/reviews/${encodeURIComponent(id)}`);
    }

    // === Rankings ===

    async getRankings(params = {}) {
        const qs = new URLSearchParams();
        if (params.limit) qs.set('limit', params.limit);
        const query = qs.toString();
        return this._fetch(`/api/rankings${query ? '?' + query : ''}`);
    }

    // === Proofs ===

    async getProofs(params = {}) {
        const qs = new URLSearchParams();
        if (params.limit) qs.set('limit', params.limit);
        if (params.verified) qs.set('verified', params.verified);
        const query = qs.toString();
        return this._fetch(`/api/proofs${query ? '?' + query : ''}`);
    }

    async getProof(id) {
        return this._fetch(`/api/proofs/${encodeURIComponent(id)}`);
    }

    // === Menu / Shop ===

    async getMenu() {
        return this._fetch('/api/menu');
    }

    async getMenuCategory(category, page = 1) {
        return this._fetch(`/api/menu/${encodeURIComponent(category)}?page=${page}`);
    }

    async getProductOptions(productId) {
        return this._fetch(`/api/products/${encodeURIComponent(productId)}/options`);
    }

    async uploadDesign(file) {
        const formData = new FormData();
        formData.append('file', file);
        const url = `${this.baseUrl}/api/designs/upload`;
        const response = await fetch(url, {
            method: 'POST',
            body: formData,
        });
        if (!response.ok) {
            const errorBody = await response.text();
            let message;
            try {
                const parsed = JSON.parse(errorBody);
                message = parsed.message || parsed.detail || `HTTP ${response.status}`;
            } catch (e) {
                message = `HTTP ${response.status}: ${errorBody.substring(0, 200)}`;
            }
            throw new Error(message);
        }
        return response.json();
    }

    async createProductOrder(data) {
        return this._post('/api/order/product', data);
    }

    async getOrder(orderId) {
        return this._fetch(`/api/order/${encodeURIComponent(orderId)}`);
    }

    async submitPayment(orderId, txId) {
        return this._fetch(`/api/order/${encodeURIComponent(orderId)}/payment`, {
            method: 'PUT',
            body: JSON.stringify({ tx_id: txId }),
        });
    }

    async submitFeedback(data) {
        return this._post('/api/feedback', data);
    }
}

// Export singleton
window.api = new APIClient();
