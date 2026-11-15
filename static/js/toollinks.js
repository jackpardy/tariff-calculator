// toollinks.js: the routine tools' one link to the competition and club pages
// ("Competitions and clubs", views/tool.templ) reads "Your competitions and
// clubs" when this browser has saved links, the same lists the hub page
// (/competitions) shows. Keep the keys in step with linkKeys in competitions.js.
(function () {
    const keys = [
        'trampolineCompetitions', 'trampolineClubs', 'trampolineMemberLinks',
        'trampolineCoachLinks', 'trampolineGymnastLinks', 'trampolineEntryLinks',
    ];

    function hasSavedLinks() {
        return keys.some((key) => {
            try {
                const links = JSON.parse(localStorage.getItem(key) || '[]');
                return Array.isArray(links) && links.some((l) => l && l.url);
            } catch (e) {
                return false;
            }
        });
    }

    document.addEventListener('DOMContentLoaded', () => {
        const links = document.querySelectorAll('[data-competitions-link]');
        if (links.length === 0 || !hasSavedLinks()) { return; }
        for (const link of links) {
            const head = link.querySelector('[data-competitions-head]');
            if (head) { head.textContent = 'Your competitions'; }
        }
    });
})();
