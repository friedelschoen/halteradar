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
    import TripCard from "../components/TripCard.svelte";
    import TripStopTable from "../components/TripStopTable.svelte";
    import type { TripStop } from "../schema/TripStop";

    export let trip: string;

    let stops: TripStop[] = [];
    let loadingStops = false;
    let loadedTrip = "";

    async function fetchJSON(url: URL | RequestInfo) {
        const res = await fetch(url);
        const json = await res.json();
        if (!res.ok) throw new Error(json?.error ?? res.statusText);
        return json.result;
    }

    async function loadStops(currentTrip: string) {
        loadingStops = true;

        try {
            stops = await fetchJSON(
                "/api/trip/" + encodeURIComponent(currentTrip) + "/stops",
            );
        } finally {
            loadingStops = false;
        }
    }

    $: if (trip && trip !== loadedTrip) {
        loadedTrip = trip;
        stops = [];
        loadStops(trip);
    }
</script>

<h2>Trip {trip}</h2>

<TripCard {trip} />

<h3>Stops</h3>

{#if loadingStops}
    <p class="muted">Loading stops...</p>
{:else if stops.length}
    <TripStopTable {stops} />
{:else}
    <p class="muted">No stops found.</p>
{/if}
