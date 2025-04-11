import { asyncWorkloadFn, AsyncWorkloadEvent, AsyncWorkloadConfig } from "@netlify/async-workloads";

export default asyncWorkloadFn((event: AsyncWorkloadEvent) => {

	console.log('Hello, Async Workloads!');

});

export const asyncWorkloadConfig: AsyncWorkloadConfig = {
	events: ['say-hello']
};
