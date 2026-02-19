/**
 * Skills Components — Reusable rendering functions
 * Skill cards, rows, review rows, search form
 */

const SkillComponents = {
    /**
     * Skill card for grid display
     */
    skillCard(skill) {
        const score = skill.avg_score != null ? Templates.scoreBadge(skill.avg_score) : '';
        const category = Templates.categoryBadge(skill.category);
        return `
            <div class="card card-hover skill-card" onclick="navigate('/skill/${encodeURIComponent(skill.id)}')">
                <div class="skill-name">${escapeHtml(skill.name)}</div>
                <div class="skill-desc">${escapeHtml(skill.description || '')}</div>
                <div class="skill-meta">
                    ${score}
                    ${category}
                    <span>${Templates.formatNumber(skill.review_count)} reviews</span>
                    <span>${Templates.formatNumber(skill.installs)} installs</span>
                </div>
            </div>`;
    },

    /**
     * Skill row for table display
     */
    skillRow(skill, rank) {
        const score = skill.avg_score != null ? Templates.scoreBadge(skill.avg_score) : '<span class="badge">-</span>';
        const security = skill.avg_security_score != null ? Templates.scoreBadge(skill.avg_security_score) : '<span class="badge">-</span>';
        const category = Templates.categoryBadge(skill.category);
        return `
            <tr style="cursor: pointer;" onclick="navigate('/skill/${encodeURIComponent(skill.id)}')">
                ${rank != null ? `<td style="color: var(--text-muted);">#${rank}</td>` : ''}
                <td>
                    <div style="font-weight: 500; color: var(--text-primary);">${escapeHtml(skill.name)}</div>
                </td>
                <td>${category}</td>
                <td>${score}</td>
                <td>${security}</td>
                <td>${Templates.formatNumber(skill.review_count)}</td>
                <td>${Templates.formatNumber(skill.installs)}</td>
            </tr>`;
    },

    /**
     * Review row
     */
    reviewRow(review) {
        const score = Templates.scoreBadge(review.score);
        const date = Templates.formatDate(review.created);
        const proofBadge = review.proof_id ? Templates.proofBadge(true) : '';

        return `
            <div class="review-item">
                <div class="review-meta">
                    ${score}
                    <span>${escapeHtml(review.agent_model || '')}</span>
                    <span>${date}</span>
                    ${proofBadge}
                </div>
                <div class="review-task">${escapeHtml(review.task || '')}</div>
                <div class="review-body">
                    ${review.what_worked ? `<div style="margin-bottom: 0.5rem;"><strong style="color: var(--green);">Worked:</strong> ${escapeHtml(review.what_worked)}</div>` : ''}
                    ${review.what_failed ? `<div><strong style="color: var(--red);">Failed:</strong> ${escapeHtml(review.what_failed)}</div>` : ''}
                </div>
            </div>`;
    },

    /**
     * Search form with filters
     */
    searchForm(params = {}) {
        const categories = ['frontend', 'backend', 'devtools', 'security', 'ai-agents', 'mobile', 'content', 'design', 'data', 'general'];
        const sorts = [
            { value: 'rank', label: 'Best Ranked' },
            { value: 'installs', label: 'Most Installs' },
            { value: 'reviews', label: 'Most Reviews' },
            { value: 'security', label: 'Best Security' },
            { value: 'newest', label: 'Newest' },
        ];

        const categoryOptions = categories.map(c =>
            `<option value="${c}" ${params.category === c ? 'selected' : ''}>${c}</option>`
        ).join('');

        const sortOptions = sorts.map(s =>
            `<option value="${s.value}" ${params.sort === s.value ? 'selected' : ''}>${s.label}</option>`
        ).join('');

        return `
            <form class="skills-search" onsubmit="event.preventDefault(); SkillPages._doSearch();">
                <input type="text" class="form-input" id="skill-search-q" placeholder="Search skills..." value="${escapeHtml(params.q || '')}">
                <select class="form-select" id="skill-search-category">
                    <option value="">All Categories</option>
                    ${categoryOptions}
                </select>
                <select class="form-select" id="skill-search-sort">
                    ${sortOptions}
                </select>
                <button type="submit" class="btn btn-primary btn-sm">Search</button>
            </form>`;
    },

    /**
     * Ranked skill row for leaderboard
     */
    rankedRow(skill, index) {
        const score = skill.avg_score != null ? Templates.scoreBadge(skill.avg_score) : '<span class="badge">-</span>';
        const rank = skill.rank_score != null ? skill.rank_score.toFixed(1) : '-';

        return `
            <tr style="cursor: pointer;" onclick="navigate('/skill/${encodeURIComponent(skill.id)}')">
                <td style="font-weight: 700; color: var(--accent);">#${index + 1}</td>
                <td>
                    <div style="font-weight: 500; color: var(--text-primary);">${escapeHtml(skill.name)}</div>
                    <div style="font-size: 0.75rem; color: var(--text-muted);">${escapeHtml((skill.description || '').substring(0, 60))}</div>
                </td>
                <td>${score}</td>
                <td>${Templates.formatNumber(skill.review_count)}</td>
                <td>${skill.verified_proofs || 0}</td>
                <td style="color: var(--text-muted);">${rank}</td>
            </tr>`;
    },
};

window.SkillComponents = SkillComponents;
