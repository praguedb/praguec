using MessagePack;

namespace Common.IntegratedEventApi.Cache.Prague.Models;

[MessagePackObject]
[DataCache(CacheClassName = nameof(BPBetTypeCache))]
[DataCacheTopic(TopicNames.BrandFromConfigs.BetType)]
[DataCacheContract(typeof(BetTypeCacheContract), Role = ContractRole.Producer)]
public sealed partial class BetType
{
    [Key(0)]
    [DataCacheKey]
    public (long EventId, long BetTypeId) Key => (EventId, BetTypeId);

    [Key(1)]
    public long EventId { get; set; }

    [Key(2)]
    public long Id { get; set; }

    [IgnoreMember]
    public long BetTypeId
    {
        get => Id;
        set => Id = value;
    }

    [Key(3)]
    public string Name { get; set; }

    [Key(6)]
    public BetTypeStatus Status { get; set; }

    [Key(7)]
    public BetTypeStatus? StatusManual { get; set; }

    [Key(8)]
    public BetTypeStatus? StatusWatchDog { get; set; }

    [Key(21)]
    public decimal? LimitManualSingle { get; set; }

    [Key(24)]
    [DataCacheFromTimestamp]
    [DataCacheIgnoreEquality]
    public long Timestamp { get; set; }

    [Key(25)]
    public List<Odd> Odds { get; set; }
}
