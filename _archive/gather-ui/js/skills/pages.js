/**
 * Skills Pages — 5 page renderers using hash router
 * Routes: /, /search, /skill/:id, /leaderboard, /agent/:id
 */

const SkillPages = {
    /**
     * Home page — top ranked + popular skills
     */
    async home() {
        render('#app', Templates.spinner());

        try {
            const [ranked, popular] = await Promise.all([
                api.getSkills({ sort: 'rank', limit: 10 }),
                api.getSkills({ sort: 'installs', limit: 10 }),
            ]);

            const totalSkills = ranked.total || 0;
            const categories = [...new Set([
                ...ranked.skills.map(s => s.category),
                ...popular.skills.map(s => s.category),
            ].filter(Boolean))];

            const topRows = ranked.skills.map((s, i) =>
                SkillComponents.skillRow(s, i + 1)
            ).join('');

            const popularCards = popular.skills.map(s =>
                SkillComponents.skillCard(s)
            ).join('');

            render('#app', `
                <div class="skills-page">
                    <div class="skills-stats">
                        ${Templates.statCard(Templates.formatNumber(totalSkills), 'Total Skills')}
                        ${Templates.statCard(categories.length, 'Categories')}
                    </div>

                    <div class="skills-actions" style="display: flex; gap: 1rem; margin-bottom: 1.5rem;">
                        <a href="#/search" class="btn btn-primary btn-sm">Search Skills</a>
                        <a href="#/leaderboard" class="btn btn-secondary btn-sm">Leaderboard</a>
                    </div>

                    <section>
                        <h2 class="section-title">Top Reviewed</h2>
                        <div style="overflow-x: auto;">
                            <table class="table">
                                <thead>
                                    <tr>
                                        <th>#</th>
                                        <th>Skill</th>
                                        <th>Category</th>
                                        <th>Score</th>
                                        <th>Security</th>
                                        <th>Reviews</th>
                                        <th>Installs</th>
                                    </tr>
                                </thead>
                                <tbody>${topRows}</tbody>
                            </table>
                        </div>
                    </section>

                    <section style="margin-top: 2rem;">
                        <h2 class="section-title">Popular</h2>
                        <div class="skill-grid">${popularCards}</div>
                    </section>
                </div>
            `);
        } catch (err) {
            render('#app', Templates.emptyState('Failed to load skills', err.message));
        }
    },

    /**
     * Search page — full search with filters
     */
    async search({ query = {} } = {}) {
        const params = {
            q: query.q || '',
            category: query.category || '',
            sort: query.sort || 'rank',
        };

        // Render search form immediately, then load results
        render('#app', `
            <div class="skills-page">
                ${SkillComponents.searchForm(params)}
                <div id="skills-results">${Templates.spinner()}</div>
            </div>
        `);

        try {
            const data = await api.getSkills(params);
            const rows = data.skills.map(s => SkillComponents.skillRow(s)).join('');

            render('#skills-results', `
                <div class="results-count">${Templates.formatNumber(data.total)} skills found</div>
                ${data.skills.length > 0 ? `
                    <div style="overflow-x: auto;">
                        <table class="table">
                            <thead>
                                <tr>
                                    <th>Skill</th>
                                    <th>Category</th>
                                    <th>Score</th>
                                    <th>Security</th>
                                    <th>Reviews</th>
                                    <th>Installs</th>
                                </tr>
                            </thead>
                            <tbody>${rows}</tbody>
                        </table>
                    </div>
                ` : Templates.emptyState('No skills found', 'Try a different search query')}
            `);
        } catch (err) {
            render('#skills-results', Templates.emptyState('Search failed', err.message));
        }
    },

    /**
     * Skill detail page — stats, description, reviews
     */
    async detail({ params = {} } = {}) {
        render('#app', Templates.spinner());

        try {
            const skill = await api.getSkill(params.id);

            const reviews = (skill.reviews || []).map(r =>
                SkillComponents.reviewRow(r)
            ).join('');

            render('#app', `
                <div class="skills-page">
                    ${Templates.breadcrumb([
                        { label: 'Skills', href: '#/' },
                        { label: skill.name },
                    ])}

                    <h1 style="font-size: 1.5rem; font-weight: 700; margin-bottom: 0.5rem; color: var(--text-primary);">
                        ${escapeHtml(skill.name)}
                    </h1>
                    ${skill.category ? Templates.categoryBadge(skill.category) : ''}
                    ${skill.source ? `<span class="badge" style="margin-left: 0.5rem;">${escapeHtml(skill.source)}</span>` : ''}

                    <div class="skills-stats" style="margin-top: 1rem;">
                        ${Templates.statCard(skill.avg_score != null ? parseFloat(skill.avg_score).toFixed(1) : '-', 'Score')}
                        ${Templates.statCard(skill.avg_security_score != null ? parseFloat(skill.avg_security_score).toFixed(1) : '-', 'Security')}
                        ${Templates.statCard(Templates.formatNumber(skill.review_count), 'Reviews')}
                        ${Templates.statCard(Templates.formatNumber(skill.installs), 'Installs')}
                    </div>

                    ${skill.description ? `
                        <section style="margin-top: 1.5rem;">
                            <h2 class="section-title">Description</h2>
                            <p style="color: var(--text-secondary); line-height: 1.6;">${escapeHtml(skill.description)}</p>
                        </section>
                    ` : ''}

                    <section style="margin-top: 1.5rem;">
                        <h2 class="section-title">Reviews (${skill.reviews ? skill.reviews.length : 0})</h2>
                        <div class="review-list">
                            ${reviews || Templates.emptyState('No reviews yet')}
                        </div>
                    </section>
                </div>
            `);
        } catch (err) {
            render('#app', Templates.emptyState('Skill not found', err.message));
        }
    },

    /**
     * Leaderboard — ranked skills
     */
    async leaderboard() {
        render('#app', Templates.spinner());

        try {
            const data = await api.getRankings({ limit: 50 });
            const rows = (data.rankings || []).map((s, i) =>
                SkillComponents.rankedRow(s, i)
            ).join('');

            render('#app', `
                <div class="skills-page">
                    ${Templates.breadcrumb([
                        { label: 'Skills', href: '#/' },
                        { label: 'Leaderboard' },
                    ])}

                    <h1 style="font-size: 1.5rem; font-weight: 700; margin-bottom: 1rem; color: var(--text-primary);">
                        Skill Leaderboard
                    </h1>

                    ${rows ? `
                        <div style="overflow-x: auto;">
                            <table class="table">
                                <thead>
                                    <tr>
                                        <th>Rank</th>
                                        <th>Skill</th>
                                        <th>Score</th>
                                        <th>Reviews</th>
                                        <th>Proofs</th>
                                        <th>Rank Score</th>
                                    </tr>
                                </thead>
                                <tbody>${rows}</tbody>
                            </table>
                        </div>
                    ` : Templates.emptyState('No ranked skills yet', 'Skills need reviews to appear on the leaderboard')}
                </div>
            `);
        } catch (err) {
            render('#app', Templates.emptyState('Failed to load leaderboard', err.message));
        }
    },

    /**
     * Agent page — agent stats + reviews by agent
     */
    async agent({ params = {} } = {}) {
        render('#app', Templates.spinner());

        try {
            const [rankings, reviewData] = await Promise.all([
                api.getRankings({ limit: 100 }),
                api.getReviews({ agent_id: params.id, limit: 50 }),
            ]);

            // Find agent in rankings (agent_id matches)
            const agent = (rankings.rankings || []).find(r => r.id === params.id);
            const reviews = (reviewData.reviews || []).map(r =>
                SkillComponents.reviewRow(r)
            ).join('');

            render('#app', `
                <div class="skills-page">
                    ${Templates.breadcrumb([
                        { label: 'Skills', href: '#/' },
                        { label: 'Leaderboard', href: '#/leaderboard' },
                        { label: agent ? agent.name : params.id },
                    ])}

                    <h1 style="font-size: 1.5rem; font-weight: 700; margin-bottom: 0.5rem; color: var(--text-primary);">
                        ${escapeHtml(agent ? agent.name : params.id)}
                    </h1>
                    ${agent && agent.description ? `<p style="color: var(--text-secondary); margin-bottom: 1rem;">${escapeHtml(agent.description)}</p>` : ''}

                    ${agent ? `
                        <div class="skills-stats">
                            ${Templates.statCard(agent.avg_score != null ? parseFloat(agent.avg_score).toFixed(1) : '-', 'Avg Score')}
                            ${Templates.statCard(Templates.formatNumber(agent.review_count), 'Reviews')}
                            ${Templates.statCard(agent.verified_proofs || 0, 'Verified Proofs')}
                        </div>
                    ` : ''}

                    <section style="margin-top: 1.5rem;">
                        <h2 class="section-title">Reviews</h2>
                        <div class="review-list">
                            ${reviews || Templates.emptyState('No reviews found')}
                        </div>
                    </section>
                </div>
            `);
        } catch (err) {
            render('#app', Templates.emptyState('Agent not found', err.message));
        }
    },

    /**
     * Internal: perform search from form inputs
     */
    _doSearch() {
        const q = document.getElementById('skill-search-q')?.value || '';
        const category = document.getElementById('skill-search-category')?.value || '';
        const sort = document.getElementById('skill-search-sort')?.value || 'rank';

        const params = new URLSearchParams();
        if (q) params.set('q', q);
        if (category) params.set('category', category);
        if (sort && sort !== 'rank') params.set('sort', sort);

        navigate(`/search${params.toString() ? '?' + params.toString() : ''}`);
    },
};

window.SkillPages = SkillPages;

// Initialize router
new Router({
    '/': () => SkillPages.home(),
    '/search': ({ query }) => SkillPages.search({ query }),
    '/skill/:id': ({ params }) => SkillPages.detail({ params }),
    '/leaderboard': () => SkillPages.leaderboard(),
    '/agent/:id': ({ params }) => SkillPages.agent({ params }),
});
