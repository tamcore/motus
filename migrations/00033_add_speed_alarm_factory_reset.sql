-- +goose Up
ALTER TABLE commands DROP CONSTRAINT IF EXISTS valid_command_type;
ALTER TABLE commands ADD CONSTRAINT valid_command_type CHECK (
    type IN ('rebootDevice','positionSingle','positionPeriodic',
             'sosNumber','custom','setSpeedAlarm','factoryReset')
);

-- +goose Down
ALTER TABLE commands DROP CONSTRAINT IF EXISTS valid_command_type;
ALTER TABLE commands ADD CONSTRAINT valid_command_type CHECK (
    type IN ('rebootDevice','positionPeriodic','positionSingle','sosNumber','custom')
);
