<!--
Copyright (C) 2026 Friedel Schön

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
-->

<script lang="ts">
    import TripTable from "../components/TripTable.svelte";
    import type { Trip } from "../schema/Trip";

    export let title: string;
    export let endpoint: string;
    export let emptyText = "No trips found.";

    let trips: Trip[] = [];
    let loading = false;

    async function fetchJSON(url: URL | RequestInfo) {
        const res = await fetch(url);
        const json = await res.json();
        if (!res.ok) throw new Error(json?.error ?? res.statusText);
        return json.result;
    }

    async function loadTrips() {
        if (!endpoint) return;

        loading = true;
        try {
            trips = await fetchJSON(endpoint);
        } finally {
            loading = false;
        }
    }

    let loadedEndpoint = "";

    $: if (endpoint && endpoint !== loadedEndpoint) {
        loadedEndpoint = endpoint;
        loadTrips();
    }
</script>

<h2>{title}</h2>

{#if loading}
    <p class="muted">Loading trips...</p>
{:else if trips.length}
    <TripTable {trips} />
{:else}
    <p class="muted">{emptyText}</p>
{/if}
