// Scoped lookup keeps a component inside its own HTML section.
export const byId = (root, id) => root.id === id ? root : root.querySelector('#' + id);
