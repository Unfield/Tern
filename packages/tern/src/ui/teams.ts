// Shared by the team screens. Internal: not exported.
import { tb } from '../runtime/i18n.svelte.js';

export type RoleOption = { value: string; label: string };

/** The built-in roles, labelled in the user's language. */
export function builtinRoles(): RoleOption[] {
	return [
		{ value: 'owner', label: tb('tern.ui.team.role.owner') },
		{ value: 'admin', label: tb('tern.ui.team.role.admin') },
		{ value: 'member', label: tb('tern.ui.team.role.member') }
	];
}

/** The label of a role, or the role itself for one of the app's own without a label. */
export function roleLabel(roles: RoleOption[], role: string): string {
	return roles.find((r) => r.value === role)?.label ?? role;
}

/** Whether a role manages members, as accounts decides it. */
export function manages(role: string | undefined): boolean {
	return role === 'owner' || role === 'admin';
}

/** The roles someone with role may give: owners any, admins all but owner. */
export function grantable(roles: RoleOption[], role: string | undefined): RoleOption[] {
	if (role === 'owner') return roles;
	if (role === 'admin') return roles.filter((r) => r.value !== 'owner');
	return [];
}

/**
 * The form-wide error of a failed post from one of a block's forms, which
 * post a hidden tern_form field naming the block: several blocks share the
 * page's form prop, and each shows only its own.
 */
export function messageFor(form: unknown, block: string): string | undefined {
	const f = form as { message?: string; values?: Record<string, unknown> } | null | undefined;
	return f?.values?.tern_form === block ? f.message : undefined;
}

/** The page's form prop for a block's own Form, or null if another block posted. */
export function formFor(form: unknown, block: string): unknown {
	const f = form as { values?: Record<string, unknown> } | null | undefined;
	return f?.values?.tern_form === block ? form : null;
}
