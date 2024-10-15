```markdown
# Scheduling System Limitations and User Responsibilities

## Important Notes on Time Handling

This scheduling system has specific limitations regarding time-related events:

1. **No Automatic DST Adjustments**: The system does not automatically adjust for 
   Daylight Saving Time (DST) transitions.

2. **No Special Leap Year Handling**: February 29th in leap years is treated like any 
   other day.

3. **Time Zone Agnostic**: All times are treated as specified in the user's local time 
   zone without conversion.

## User Responsibilities

Users are responsible for:

- Adjusting pipeline schedules around DST changes in their respective time zones.
- Handling February 29th schedules in leap years if needed.
- Setting critical job schedules at hours not typically affected by DST (e.g., 11:00 AM).

## Best Practices

- Always specify times in your local time zone.
- For recurring jobs, consider using times that are unaffected by DST transitions.
- Regularly review and adjust schedules, especially before time changes.

## Testing Approach

The tests in this file ensure the system behaves correctly given these limitations 
and user responsibilities. They do not attempt to solve DST or leap year issues 
programmatically.

For more detailed guidelines, please refer to the full user documentation.
```