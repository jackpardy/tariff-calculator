// sets.js: the requirement sets saved in this browser (ADR 0003), shared by the
// calculator, the tariff sheet and the requirements page. Stored as a list of
// {id, set} under 'trampolineRequirementSets', where set is the set's JSON.
const SetStore = (() => {
    const key = 'trampolineRequirementSets';

    function load() {
        try {
            const sets = JSON.parse(localStorage.getItem(key) || '[]');
            return Array.isArray(sets) ? sets.filter((s) => s && s.id && s.set) : [];
        } catch (e) {
            return [];
        }
    }

    function save(sets) {
        try { localStorage.setItem(key, JSON.stringify(sets)); } catch (e) { console.error('Could not save requirement sets:', e); }
    }

    const newId = () => 'set-' + Date.now().toString(36) + Math.random().toString(36).slice(2, 6);

    // payload is what the server needs to check a routine: a built-in reference
    // ("builtin:<id>") as it is, or a saved set's JSON ('' for none or unknown).
    function payload(ref) {
        if (!ref) { return ''; }
        if (ref.startsWith('builtin:')) { return ref; }
        const saved = load().find((s) => s.id === ref);
        return saved ? JSON.stringify(saved.set) : '';
    }

    return { load, save, newId, payload };
})();
