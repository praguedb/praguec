using MessagePack;
using MongoDB.Bson.Serialization.Attributes;

namespace Common.EventApi.Models.Branded;

/// <summary>
/// Reference to static data
/// </summary>
[MessagePackObject]
[BsonIgnoreExtraElements]
public class StaticDataRef : BaseProviderObject
{

}